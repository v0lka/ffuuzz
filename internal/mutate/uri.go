package mutate

import (
	"math/rand"
	"net/url"
	"strings"

	"ffuuzz/internal/model"
)

// reservedChars are URI-significant characters for injection testing.
var reservedChars = []string{":", "/", "?", "#", "[", "]", "@", "!", "$", "&", "'", "(", ")", "*", "+", ",", ";", "="}

// invalidPercentEncodings are malformed percent-encoded sequences.
var invalidPercentEncodings = []string{"%", "%0", "%ZZ", "%0G", "%G0", "%%41", "%2541"}

// URIMutator applies mutations to URL path and query parameters.
type URIMutator struct {
	MaxURLLen  int
	EnabledOps []string // nil or empty = all enabled
}

func (m *URIMutator) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
	ops := resolveOps(m.EnabledOps, allURIOps)
	if len(ops) == 0 {
		return MutationResult{Exchange: ex, Operators: []string{"uri:noop"}}
	}

	opName := "uri:" + ops[rng.Intn(len(ops))]

	switch opName {
	case "uri:path_segment":
		ex = m.mutatePathSegments(ex, rng)
	case "uri:query_param":
		ex = m.mutateQueryParams(ex, rng)
	case "uri:reserved_inject":
		ex = m.injectReservedChars(ex, rng)
	case "uri:percent_encoding":
		ex = m.injectInvalidEncoding(ex, rng)
	case "uri:slash_manipulation":
		ex = m.slashManipulation(ex, rng)
	case "uri:long_value":
		ex = m.longValue(ex, rng)
	}

	return MutationResult{Exchange: ex, Operators: []string{opName}}
}

func (m *URIMutator) mutatePathSegments(ex model.Exchange, rng *rand.Rand) model.Exchange {
	segments := strings.Split(strings.TrimPrefix(ex.Request.Path, "/"), "/")
	if len(segments) == 0 {
		return ex
	}

	op := rng.Intn(4)
	switch op {
	case 0: // insert random segment
		idx := rng.Intn(len(segments) + 1)
		seg := randomString(rng, 3+rng.Intn(12))
		newSegs := make([]string, 0, len(segments)+1)
		newSegs = append(newSegs, segments[:idx]...)
		newSegs = append(newSegs, seg)
		newSegs = append(newSegs, segments[idx:]...)
		segments = newSegs
	case 1: // delete a segment
		if len(segments) > 1 {
			idx := rng.Intn(len(segments))
			segments = append(segments[:idx], segments[idx+1:]...)
		}
	case 2: // duplicate a segment
		idx := rng.Intn(len(segments))
		newSegs := make([]string, 0, len(segments)+1)
		newSegs = append(newSegs, segments[:idx+1]...)
		newSegs = append(newSegs, segments[idx])
		newSegs = append(newSegs, segments[idx+1:]...)
		segments = newSegs
	case 3: // empty a segment
		idx := rng.Intn(len(segments))
		segments[idx] = ""
	}

	ex.Request.Path = "/" + strings.Join(segments, "/")
	return ex
}

func (m *URIMutator) mutateQueryParams(ex model.Exchange, rng *rand.Rand) model.Exchange {
	params, err := url.ParseQuery(ex.Request.Query)
	if err != nil {
		params = url.Values{}
	}

	op := rng.Intn(4)
	switch op {
	case 0: // add new param
		key := randomString(rng, 3+rng.Intn(8))
		params.Add(key, randomString(rng, 5+rng.Intn(20)))
	case 1: // duplicate existing key
		keys := mapKeys(params)
		if len(keys) > 0 {
			key := keys[rng.Intn(len(keys))]
			params.Add(key, params.Get(key))
		}
	case 2: // empty value
		keys := mapKeys(params)
		if len(keys) > 0 {
			key := keys[rng.Intn(len(keys))]
			params.Set(key, "")
		}
	case 3: // long value
		keys := mapKeys(params)
		if len(keys) > 0 {
			key := keys[rng.Intn(len(keys))]
			params.Set(key, strings.Repeat("A", 256+rng.Intn(512)))
		} else {
			params.Add("fuzz", strings.Repeat("A", 256+rng.Intn(512)))
		}
	}

	ex.Request.Query = params.Encode()
	return ex
}

func (m *URIMutator) injectReservedChars(ex model.Exchange, rng *rand.Rand) model.Exchange {
	ch := reservedChars[rng.Intn(len(reservedChars))]
	if rng.Intn(2) == 0 {
		// Inject into path
		pos := rng.Intn(len(ex.Request.Path) + 1)
		ex.Request.Path = ex.Request.Path[:pos] + ch + ex.Request.Path[pos:]
	} else {
		// Inject into query
		if ex.Request.Query == "" {
			ex.Request.Query = "x=" + url.QueryEscape(ch)
		} else {
			pos := rng.Intn(len(ex.Request.Query) + 1)
			ex.Request.Query = ex.Request.Query[:pos] + ch + ex.Request.Query[pos:]
		}
	}
	return ex
}

func (m *URIMutator) injectInvalidEncoding(ex model.Exchange, rng *rand.Rand) model.Exchange {
	enc := invalidPercentEncodings[rng.Intn(len(invalidPercentEncodings))]
	if rng.Intn(2) == 0 && len(ex.Request.Path) > 1 {
		pos := rng.Intn(len(ex.Request.Path))
		ex.Request.Path = ex.Request.Path[:pos] + enc + ex.Request.Path[pos:]
	} else {
		if ex.Request.Query == "" {
			ex.Request.Query = "q=" + enc
		} else {
			pos := rng.Intn(len(ex.Request.Query))
			ex.Request.Query = ex.Request.Query[:pos] + enc + ex.Request.Query[pos:]
		}
	}
	return ex
}

func (m *URIMutator) slashManipulation(ex model.Exchange, rng *rand.Rand) model.Exchange {
	op := rng.Intn(4)
	switch op {
	case 0: // trailing slash toggle
		if strings.HasSuffix(ex.Request.Path, "/") {
			ex.Request.Path = strings.TrimRight(ex.Request.Path, "/")
		} else {
			ex.Request.Path += "/"
		}
	case 1: // double slashes
		ex.Request.Path = strings.Replace(ex.Request.Path, "/", "//", 1)
	case 2: // dot segment
		ex.Request.Path = ex.Request.Path + "/.."
	case 3: // encoded slash
		ex.Request.Path = strings.Replace(ex.Request.Path, "/", "%2F", 1)
	}
	return ex
}

func (m *URIMutator) longValue(ex model.Exchange, rng *rand.Rand) model.Exchange {
	maxLen := m.MaxURLLen
	if maxLen <= 0 {
		maxLen = 8192
	}
	longSeg := strings.Repeat("A", maxLen/2+rng.Intn(maxLen/2))
	ex.Request.Path = ex.Request.Path + "/" + longSeg
	return ex
}

// randomString returns a random alphanumeric string of length n using the
// given RNG to select characters.
func randomString(rng *rand.Rand, n int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rng.Intn(len(charset))]
	}
	return string(b)
}
