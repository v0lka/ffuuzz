package mutate

import (
	"math/rand"
	"strings"

	"ffuuzz/internal/model"
)

// builtinHeaderDict provides common header names and fuzz-interesting values.
var builtinHeaderDict = map[string][]string{
	"Content-Type":     {"text/html", "application/xml", "multipart/form-data", "text/plain", "application/octet-stream", "application/x-www-form-urlencoded"},
	"Accept":           {"*/*", "text/html", "application/xml", "application/json", "image/webp"},
	"Authorization":    {"Bearer AAAA", "Basic dGVzdDp0ZXN0", "Bearer " + strings.Repeat("A", 512), ""},
	"X-Forwarded-For":  {"127.0.0.1", "::1", "0.0.0.0", strings.Repeat("1", 256)},
	"X-Forwarded-Host": {"localhost", "evil.com", strings.Repeat("a", 256) + ".com"},
	"User-Agent":       {"", strings.Repeat("X", 4096), "Mozilla/5.0 (compatible; MSIE 6.0)"},
	"Referer":          {"", "https://evil.com", "javascript:alert(1)"},
	"Cookie":           {"a=b", strings.Repeat("c=d; ", 200), ""},
}

// HeaderMutator applies mutations to HTTP request headers.
type HeaderMutator struct {
	MaxHdrLen   int
	UserDict    *Dictionary
	EnabledOps  []string // nil or empty = all enabled
}

func (m *HeaderMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
	if ex.Request.Headers == nil {
		ex.Request.Headers = make(map[string][]string)
	}

	ops := resolveOps(m.EnabledOps, allHeaderOps)
	if len(ops) == 0 {
		return MutationResult{Exchange: ex, Operators: []string{"header:noop"}}
	}

	opName := "header:" + ops[rng.Intn(len(ops))]

	switch opName {
	case "header:add":
		ex = m.addHeader(ex, rng)
	case "header:remove":
		ex = m.removeHeader(ex, rng)
	case "header:duplicate":
		ex = m.duplicateHeader(ex, rng)
	case "header:long_value":
		ex = m.longValue(ex, rng)
	case "header:dict_substitute":
		ex = m.dictSubstitute(ex, rng)
	case "header:conflicting":
		ex = m.conflicting(ex, rng)
	}

	return MutationResult{Exchange: ex, Operators: []string{opName}}
}

func (m *HeaderMutator) addHeader(ex model.Exchange, rng *rand.Rand) model.Exchange {
	name := randomString(rng, 5+rng.Intn(10))
	value := randomString(rng, 10+rng.Intn(50))
	ex.Request.Headers["X-Fuzz-"+name] = []string{value}
	return ex
}

func (m *HeaderMutator) removeHeader(ex model.Exchange, rng *rand.Rand) model.Exchange {
	keys := mapKeys(ex.Request.Headers)
	if len(keys) == 0 {
		return ex
	}
	key := keys[rng.Intn(len(keys))]
	delete(ex.Request.Headers, key)
	return ex
}

func (m *HeaderMutator) duplicateHeader(ex model.Exchange, rng *rand.Rand) model.Exchange {
	keys := mapKeys(ex.Request.Headers)
	if len(keys) == 0 {
		return ex
	}
	key := keys[rng.Intn(len(keys))]
	vals := ex.Request.Headers[key]
	if len(vals) > 0 {
		ex.Request.Headers[key] = append(vals, vals[0])
	}
	return ex
}

func (m *HeaderMutator) longValue(ex model.Exchange, rng *rand.Rand) model.Exchange {
	maxLen := m.MaxHdrLen
	if maxLen <= 0 {
		maxLen = 8192
	}
	keys := mapKeys(ex.Request.Headers)
	if len(keys) == 0 {
		ex.Request.Headers["X-Fuzz-Long"] = []string{strings.Repeat("B", maxLen)}
		return ex
	}
	key := keys[rng.Intn(len(keys))]
	ex.Request.Headers[key] = []string{strings.Repeat("B", maxLen/2+rng.Intn(maxLen/2))}
	return ex
}

func (m *HeaderMutator) dictSubstitute(ex model.Exchange, rng *rand.Rand) model.Exchange {
	// Merge built-in and user dictionaries
	dict := make(map[string][]string)
	for k, v := range builtinHeaderDict {
		dict[k] = v
	}
	if m.UserDict != nil {
		// Get endpoint-aware values from the user dictionary
		endpoint := ex.Request.Path
		userHeaders := mergeUserDict(builtinHeaderDict, m.UserDict, endpoint)
		for k, v := range userHeaders {
			dict[k] = v
		}
	}

	// Pick a random header from the dictionary
	dictKeys := make([]string, 0, len(dict))
	for k := range dict {
		dictKeys = append(dictKeys, k)
	}
	key := dictKeys[rng.Intn(len(dictKeys))]
	vals := dict[key]
	val := vals[rng.Intn(len(vals))]
	ex.Request.Headers[key] = []string{val}
	return ex
}

// mergeUserDict returns endpoint-aware header→values from the user dictionary,
// merging user-supplied entries with built-in dict values.
func mergeUserDict(builtin map[string][]string, userDict *Dictionary, endpoint string) map[string][]string {
	result := make(map[string][]string)
	for k, v := range builtin {
		result[k] = make([]string, len(v))
		copy(result[k], v)
	}
	// Enrich with all user dictionary headers for this endpoint
	for _, h := range userDict.AllHeaders(endpoint) {
		userVals := userDict.ValuesForHeader(endpoint, h)
		if _, exists := result[h]; exists {
			result[h] = append(result[h], userVals...)
		} else {
			dest := make([]string, len(userVals))
			copy(dest, userVals)
			result[h] = dest
		}
	}
	return result
}

func (m *HeaderMutator) conflicting(ex model.Exchange, rng *rand.Rand) model.Exchange {
	// Inject conflicting Content-Length + Transfer-Encoding
	op := rng.Intn(3)
	switch op {
	case 0:
		ex.Request.Headers["Content-Length"] = []string{"0"}
		ex.Request.Headers["Transfer-Encoding"] = []string{"chunked"}
	case 1:
		ex.Request.Headers["Content-Length"] = []string{"999999"}
	case 2:
		ex.Request.Headers["Transfer-Encoding"] = []string{"chunked, identity"}
	}
	return ex
}
