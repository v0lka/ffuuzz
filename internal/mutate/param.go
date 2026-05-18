package mutate

import (
	"encoding/base64"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"ffuuzz/internal/model"
)

// ParamMutator injects fuzz payloads into query parameter values (GET) and
// form-encoded body parameter values (POST application/x-www-form-urlencoded).
type ParamMutator struct{}

func (m *ParamMutator) Mutate(ex model.Exchange, rng *rand.Rand, _ float64) MutationResult {
	queryParams, err := url.ParseQuery(ex.Request.Query)
	if err != nil {
		queryParams = url.Values{}
	}
	formParams, hasForm := m.parseFormBody(ex)

	type surface struct {
		name   string
		params url.Values
	}
	var candidates []surface
	if len(queryParams) > 0 {
		candidates = append(candidates, surface{name: "query", params: queryParams})
	}
	if hasForm && len(formParams) > 0 {
		candidates = append(candidates, surface{name: "form", params: formParams})
	}

	// Fallback: no existing params anywhere — add a new query param with a fuzz value.
	if len(candidates) == 0 {
		queryParams.Set("fuzz", fuzzStrings[rng.Intn(len(fuzzStrings))])
		ex.Request.Query = queryParams.Encode()
		return MutationResult{Exchange: ex, Operators: []string{"param:string_mutation"}}
	}

	// Pick a random surface and inject a fuzz string into a random param.
	target := candidates[rng.Intn(len(candidates))]
	keys := mapKeys(target.params)
	key := keys[rng.Intn(len(keys))]
	target.params.Set(key, fuzzStrings[rng.Intn(len(fuzzStrings))])

	switch target.name {
	case "query":
		ex.Request.Query = target.params.Encode()
	case "form":
		ex.Request.BodyB64 = base64.StdEncoding.EncodeToString([]byte(target.params.Encode()))
	}

	return MutationResult{Exchange: ex, Operators: []string{"param:string_mutation"}}
}

// parseFormBody decodes and parses the request body as url.Values when the
// Content-Type indicates application/x-www-form-urlencoded.
func (m *ParamMutator) parseFormBody(ex model.Exchange) (url.Values, bool) {
	ct := http.Header(ex.Request.Headers).Get("Content-Type")
	if !strings.Contains(ct, "form-urlencoded") {
		return nil, false
	}
	if ex.Request.BodyB64 == "" {
		return nil, false
	}
	bodyBytes, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
	if err != nil || len(bodyBytes) == 0 {
		return nil, false
	}
	params, err := url.ParseQuery(string(bodyBytes))
	if err != nil {
		return nil, false
	}
	return params, true
}
