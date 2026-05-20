package mutate

import (
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"net/url"
	"strings"
	"testing"

	"ffuuzz/internal/model"
)

func newRNG(seed int64) *rand.Rand {
	return rand.New(rand.NewSource(seed))
}

func makeEx(method, path, query string) model.Exchange {
	return model.Exchange{
		Request: model.RequestData{
			Method:  method,
			Path:    path,
			Query:   query,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
		},
	}
}

func makeExWithBody(method, path string, body interface{}) model.Exchange {
	data, _ := json.Marshal(body)
	return model.Exchange{
		Request: model.RequestData{
			Method:  method,
			Path:    path,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			BodyB64: base64.StdEncoding.EncodeToString(data),
		},
	}
}

func TestURIMutator_AllOps(t *testing.T) {
	m := &URIMutator{MaxURLLen: 8192}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api/users/123", "page=1&limit=10")
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected at least one operator")
		}
	}
}

func TestURIMutator_EmptyPath(t *testing.T) {
	m := &URIMutator{MaxURLLen: 8192}
	rng := newRNG(42)
	ex := makeEx("GET", "/", "")
	result := m.Mutate(ex, rng, 1.0)
	if len(result.Operators) == 0 {
		t.Error("expected operator for empty path")
	}
}

func TestURIMutator_NoQuery(t *testing.T) {
	m := &URIMutator{MaxURLLen: 8192}
	rng := newRNG(0)
	ex := makeEx("GET", "/api/test", "")
	result := m.Mutate(ex, rng, 1.0)
	if result.Exchange.Request.Path == "" {
		t.Error("path should not be empty after mutation")
	}
}

func TestURIMutator_LongValue(t *testing.T) {
	m := &URIMutator{MaxURLLen: 100}
	ex := makeEx("GET", "/api", "")
	// Force longValue op (case 5)
	for seed := int64(0); seed < 100; seed++ {
		rng := newRNG(seed)
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected operator")
		}
	}
}

func TestHeaderMutator_AllOps(t *testing.T) {
	m := &HeaderMutator{MaxHdrLen: 8192}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api", "")
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected at least one operator")
		}
	}
}

func TestHeaderMutator_NilHeaders(t *testing.T) {
	m := &HeaderMutator{MaxHdrLen: 8192}
	rng := newRNG(42)
	ex := model.Exchange{
		Request: model.RequestData{
			Method:  "GET",
			Path:    "/api",
			Headers: nil,
		},
	}
	result := m.Mutate(ex, rng, 1.0)
	if result.Exchange.Request.Headers == nil {
		t.Error("headers should be initialized")
	}
}

func TestHeaderMutator_UserDict(t *testing.T) {
	userDict := NewDictionary()
	userDict.AddGlobal("X-Custom", []string{"val1", "val2"})
	m := &HeaderMutator{MaxHdrLen: 8192, UserDict: userDict}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api", "")
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected operator")
		}
	}
}

func TestHeaderMutator_EmptyHeaders(t *testing.T) {
	m := &HeaderMutator{MaxHdrLen: 8192}
	rng := newRNG(0)
	ex := model.Exchange{
		Request: model.RequestData{
			Method:  "GET",
			Path:    "/api",
			Headers: map[string][]string{},
		},
	}
	result := m.Mutate(ex, rng, 1.0)
	if len(result.Operators) == 0 {
		t.Error("expected operator")
	}
}

func TestJSONMutator_ValidJSON(t *testing.T) {
	m := &JSONMutator{MaxBodyLen: 1 << 20}
	body := map[string]interface{}{"name": "test", "age": 42, "tags": []interface{}{"a", "b"}}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api", body)
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected operator")
		}
		if result.Exchange.Request.BodyB64 == "" {
			t.Error("body should not be empty after mutation")
		}
	}
}

func TestJSONMutator_EmptyBody(t *testing.T) {
	m := &JSONMutator{MaxBodyLen: 1 << 20}
	rng := newRNG(42)
	ex := model.Exchange{
		Request: model.RequestData{
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			BodyB64: "",
		},
	}
	result := m.Mutate(ex, rng, 1.0)
	if result.Operators[0] != "json:noop" {
		t.Errorf("expected json:noop, got %v", result.Operators)
	}
}

func TestJSONMutator_NonJSONContentType(t *testing.T) {
	m := &JSONMutator{MaxBodyLen: 1 << 20}
	rng := newRNG(42)
	data, _ := json.Marshal(map[string]interface{}{"key": "val"})
	ex := model.Exchange{
		Request: model.RequestData{
			Headers: map[string][]string{"Content-Type": {"text/plain"}},
			BodyB64: base64.StdEncoding.EncodeToString(data),
		},
	}
	result := m.Mutate(ex, rng, 1.0)
	// Should fallback to primitive mutation
	if len(result.Operators) == 0 {
		t.Error("expected operator")
	}
}

func TestJSONMutator_ArrayBody(t *testing.T) {
	m := &JSONMutator{MaxBodyLen: 1 << 20}
	body := []interface{}{1, "two", 3.0, nil}
	for seed := int64(0); seed < 30; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api", body)
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected operator")
		}
	}
}

func TestJSONMutator_NestedObject(t *testing.T) {
	m := &JSONMutator{MaxBodyLen: 1 << 20}
	body := map[string]interface{}{
		"user": map[string]interface{}{"name": "test"},
		"items": []interface{}{
			map[string]interface{}{"id": 1},
		},
	}
	for seed := int64(0); seed < 30; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api", body)
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected operator")
		}
	}
}

func TestSeqMutator_AllOps(t *testing.T) {
	m := &SeqMutator{}
	exs := []model.Exchange{
		makeEx("GET", "/a", ""),
		makeEx("POST", "/b", ""),
		makeEx("PUT", "/c", ""),
	}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		result := m.Mutate(exs, rng, 0.5)
		if len(result.Operators) == 0 {
			t.Error("expected operator")
		}
	}
}

func TestSeqMutator_SingleExchange(t *testing.T) {
	m := &SeqMutator{}
	exs := []model.Exchange{makeEx("GET", "/a", "")}
	rng := newRNG(42)
	result := m.Mutate(exs, rng, 0.5)
	if result.Operators[0] != "seq:noop" {
		t.Errorf("expected seq:noop, got %v", result.Operators)
	}
	if len(result.Exchanges) != 1 {
		t.Errorf("expected 1 exchange, got %d", len(result.Exchanges))
	}
}

func TestSeqDrop(t *testing.T) {
	exs := []model.Exchange{
		makeEx("GET", "/a", ""),
		makeEx("POST", "/b", ""),
		makeEx("PUT", "/c", ""),
	}
	rng := newRNG(42)
	result := SeqDrop(exs, rng)
	if len(result) != 2 {
		t.Fatalf("expected 2 exchanges after drop, got %d", len(result))
	}
	// First exchange should always be preserved
	if result[0].Request.Path != "/a" {
		t.Error("first exchange should be preserved")
	}
}

func TestSeqDrop_SingleExchange(t *testing.T) {
	exs := []model.Exchange{makeEx("GET", "/a", "")}
	rng := newRNG(42)
	result := SeqDrop(exs, rng)
	if len(result) != 1 {
		t.Fatalf("expected 1 exchange, got %d", len(result))
	}
}

func TestSeqDuplicate(t *testing.T) {
	exs := []model.Exchange{
		makeEx("GET", "/a", ""),
		makeEx("POST", "/b", ""),
	}
	rng := newRNG(42)
	result := SeqDuplicate(exs, rng)
	if len(result) != 3 {
		t.Fatalf("expected 3 exchanges after duplicate, got %d", len(result))
	}
}

func TestSeqDuplicate_Empty(t *testing.T) {
	var exs []model.Exchange
	rng := newRNG(42)
	result := SeqDuplicate(exs, rng)
	if len(result) != 0 {
		t.Fatalf("expected 0 exchanges, got %d", len(result))
	}
}

func TestSeqSwap(t *testing.T) {
	exs := []model.Exchange{
		makeEx("GET", "/a", ""),
		makeEx("POST", "/b", ""),
		makeEx("PUT", "/c", ""),
	}
	rng := newRNG(42)
	result := SeqSwap(exs, rng)
	if len(result) != 3 {
		t.Fatalf("expected 3 exchanges, got %d", len(result))
	}
}

func TestSeqSwap_SingleExchange(t *testing.T) {
	exs := []model.Exchange{makeEx("GET", "/a", "")}
	rng := newRNG(42)
	result := SeqSwap(exs, rng)
	if len(result) != 1 {
		t.Fatalf("expected 1 exchange, got %d", len(result))
	}
}

func TestNewPipeline(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPipeline(cfg)
	if p == nil {
		t.Fatal("expected non-nil Pipeline")
	}
	if p.Intensity() != 0.5 {
		t.Errorf("Intensity = %f, want 0.5", p.Intensity())
	}
}

func TestPipeline_MutateAppliesOperators(t *testing.T) {
	cfg := DefaultConfig()
	p := NewPipeline(cfg)
	body := map[string]interface{}{"key": "value"}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api/test", body)
		result := p.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected at least one operator")
		}
	}
}

func TestPipeline_FallbackToPrimitive(t *testing.T) {
	// With intensity=0, no mutators activate, so primitive fallback
	cfg := DefaultConfig()
	p := NewPipeline(cfg)
	ex := makeEx("GET", "/api", "")
	rng := newRNG(42)
	result := p.Mutate(ex, rng, 0.0)
	if len(result.Operators) == 0 {
		t.Error("expected primitive fallback operator")
	}
}

func TestPipeline_SizeLimits(t *testing.T) {
	cfg := Config{
		PathQuery:  true,
		Headers:    true,
		JSONBody:   true,
		Intensity:  1.0,
		MaxURLLen:  50,
		MaxHdrLen:  50,
		MaxBodyLen: 50,
	}
	p := NewPipeline(cfg)
	bigBody := map[string]interface{}{"key": string(make([]byte, 200))}
	for seed := int64(0); seed < 20; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api/test/with/a/long/path/segment", bigBody)
		result := p.Mutate(ex, rng, 1.0)
		combined := result.Exchange.Request.Path
		if result.Exchange.Request.Query != "" {
			combined += "?" + result.Exchange.Request.Query
		}
		if len(combined) > cfg.MaxURLLen+1000 {
			// Allow some slack for mutation adding chars
			t.Errorf("combined length %d > MaxURLLen+1000", len(combined))
		}
	}
}

func TestPipeline_DisabledMutators(t *testing.T) {
	cfg := Config{
		PathQuery: false,
		Headers:   false,
		JSONBody:  false,
		Sequence:  false,
		Intensity: 0.5,
	}
	p := NewPipeline(cfg)
	rng := newRNG(42)
	ex := makeEx("GET", "/api", "")
	result := p.Mutate(ex, rng, 1.0)
	// Should fallback to primitive
	if len(result.Operators) == 0 {
		t.Error("expected primitive fallback")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.PathQuery {
		t.Error("expected PathQuery=true")
	}
	if !cfg.Headers {
		t.Error("expected Headers=true")
	}
	if !cfg.JSONBody {
		t.Error("expected JSONBody=true")
	}
	if !cfg.Sequence {
		t.Error("expected Sequence=true")
	}
	if !cfg.Params {
		t.Error("expected Params=true")
	}
	if cfg.Intensity != 0.5 {
		t.Errorf("Intensity = %f, want 0.5", cfg.Intensity)
	}
	if cfg.MaxURLLen != 8192 {
		t.Errorf("MaxURLLen = %d", cfg.MaxURLLen)
	}
}

func TestMapKeys(t *testing.T) {
	m := map[string][]string{"a": {"1"}, "b": {"2"}, "c": {"3"}}
	keys := mapKeys(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
}

func TestMapKeys_Empty(t *testing.T) {
	keys := mapKeys(map[string][]string{})
	if len(keys) != 0 {
		t.Fatalf("expected 0 keys, got %d", len(keys))
	}
}

func TestCopyExchanges(t *testing.T) {
	exs := []model.Exchange{makeEx("GET", "/a", ""), makeEx("POST", "/b", "")}
	copied := copyExchanges(exs)
	if len(copied) != 2 {
		t.Fatalf("expected 2, got %d", len(copied))
	}
	// Modify original, copied should be unaffected
	exs[0].Request.Path = "/modified"
	if copied[0].Request.Path == "/modified" {
		t.Error("copy should be independent")
	}
}

func TestEnforceSizeLimits_LongPath(t *testing.T) {
	cfg := Config{MaxURLLen: 20, MaxHdrLen: 100, MaxBodyLen: 100}
	p := NewPipeline(cfg)
	ex := model.Exchange{
		Request: model.RequestData{
			Path:    "/a/very/long/path/that/exceeds/the/limit",
			Headers: map[string][]string{},
		},
	}
	result := p.enforceSizeLimits(ex)
	if len(result.Request.Path) > 20 {
		t.Errorf("path length = %d, should be <= 20", len(result.Request.Path))
	}
}

func TestEnforceSizeLimits_LongQuery(t *testing.T) {
	cfg := Config{MaxURLLen: 30, MaxHdrLen: 100, MaxBodyLen: 100}
	p := NewPipeline(cfg)
	ex := model.Exchange{
		Request: model.RequestData{
			Path:    "/api",
			Query:   "a=very_long_query_string_that_exceeds",
			Headers: map[string][]string{},
		},
	}
	result := p.enforceSizeLimits(ex)
	combined := result.Request.Path
	if result.Request.Query != "" {
		combined += "?" + result.Request.Query
	}
	if len(combined) > 30+1 {
		// +1 for the "?" separator
		t.Errorf("combined length %d > 31", len(combined))
	}
}

func TestEnforceSizeLimits_LongHeader(t *testing.T) {
	cfg := Config{MaxURLLen: 100, MaxHdrLen: 10, MaxBodyLen: 100}
	p := NewPipeline(cfg)
	ex := model.Exchange{
		Request: model.RequestData{
			Path:    "/api",
			Headers: map[string][]string{"X-Long": {"abcdefghijklmnop"}},
		},
	}
	result := p.enforceSizeLimits(ex)
	if len(result.Request.Headers["X-Long"][0]) > 10 {
		t.Errorf("header value length = %d, should be <= 10", len(result.Request.Headers["X-Long"][0]))
	}
}

func TestEnforceSizeLimits_LongBody(t *testing.T) {
	cfg := Config{MaxURLLen: 100, MaxHdrLen: 100, MaxBodyLen: 10}
	p := NewPipeline(cfg)
	bigData := make([]byte, 100)
	for i := range bigData {
		bigData[i] = 'A'
	}
	ex := model.Exchange{
		Request: model.RequestData{
			Path:    "/api",
			Headers: map[string][]string{},
			BodyB64: base64.StdEncoding.EncodeToString(bigData),
		},
	}
	result := p.enforceSizeLimits(ex)
	decoded, _ := base64.StdEncoding.DecodeString(result.Request.BodyB64)
	if len(decoded) > 10 {
		t.Errorf("body length = %d, should be <= 10", len(decoded))
	}
	if !result.Request.BodyTruncated {
		t.Error("expected BodyTruncated=true")
	}
}

func TestRandomString(t *testing.T) {
	rng := newRNG(42)
	s := randomString(rng, 10)
	if len(s) != 10 {
		t.Errorf("len = %d, want 10", len(s))
	}
	// All chars should be from charset
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			t.Errorf("unexpected char: %c", c)
		}
	}
}

func TestExchangeMutatorFunc(t *testing.T) {
	called := false
	fn := ExchangeMutatorFunc(func(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
		called = true
		return MutationResult{Exchange: ex, Operators: []string{"test"}}
	})
	rng := newRNG(0)
	result := fn.Mutate(model.Exchange{}, rng, 1.0)
	if !called {
		t.Error("expected function to be called")
	}
	if result.Operators[0] != "test" {
		t.Errorf("operator = %q", result.Operators[0])
	}
}

func makeExWithFormBody(method, path, query, formBody string) model.Exchange {
	return model.Exchange{
		Request: model.RequestData{
			Method:  method,
			Path:    path,
			Query:   query,
			Headers: map[string][]string{"Content-Type": {"application/x-www-form-urlencoded"}},
			BodyB64: base64.StdEncoding.EncodeToString([]byte(formBody)),
		},
	}
}

func isFuzzString(s string) bool {
	for _, fs := range fuzzStrings {
		if s == fs {
			return true
		}
	}
	return false
}

func TestParamMutator_QueryParams(t *testing.T) {
	m := &ParamMutator{}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api/search", "user=alice&role=admin")
		result := m.Mutate(ex, rng, 1.0)
		if result.Operators[0] != "param:string_mutation" {
			t.Errorf("seed %d: expected param:string_mutation, got %v", seed, result.Operators)
		}
		params, err := url.ParseQuery(result.Exchange.Request.Query)
		if err != nil {
			t.Fatalf("seed %d: invalid query string: %v", seed, err)
		}
		// At least one value should be a fuzz string
		found := false
		for _, vals := range params {
			for _, v := range vals {
				if isFuzzString(v) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("seed %d: no fuzz string found in query params: %v", seed, params)
		}
	}
}

func TestParamMutator_FormBody(t *testing.T) {
	m := &ParamMutator{}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeExWithFormBody("POST", "/login", "", "username=admin&password=secret")
		result := m.Mutate(ex, rng, 1.0)
		if result.Operators[0] != "param:string_mutation" {
			t.Errorf("seed %d: expected param:string_mutation, got %v", seed, result.Operators)
		}
		bodyBytes, err := base64.StdEncoding.DecodeString(result.Exchange.Request.BodyB64)
		if err != nil {
			t.Fatalf("seed %d: invalid base64 body: %v", seed, err)
		}
		params, err := url.ParseQuery(string(bodyBytes))
		if err != nil {
			t.Fatalf("seed %d: invalid form body: %v", seed, err)
		}
		found := false
		for _, vals := range params {
			for _, v := range vals {
				if isFuzzString(v) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("seed %d: no fuzz string found in form body: %v", seed, params)
		}
	}
}

func TestParamMutator_NoParams(t *testing.T) {
	m := &ParamMutator{}
	rng := newRNG(42)
	ex := model.Exchange{
		Request: model.RequestData{
			Method:  "GET",
			Path:    "/api",
			Headers: map[string][]string{},
		},
	}
	result := m.Mutate(ex, rng, 1.0)
	if result.Operators[0] != "param:string_mutation" {
		t.Errorf("expected param:string_mutation, got %v", result.Operators)
	}
	// Fallback should add a "fuzz" query param
	params, _ := url.ParseQuery(result.Exchange.Request.Query)
	if len(params) == 0 {
		t.Error("expected at least one query param after fallback")
	}
	fuzzVal := params.Get("fuzz")
	if !isFuzzString(fuzzVal) {
		t.Errorf("fallback param value %q is not a fuzz string", fuzzVal)
	}
}

func TestParamMutator_BothSurfaces(t *testing.T) {
	m := &ParamMutator{}
	queryHit := false
	formHit := false
	for seed := int64(0); seed < 200; seed++ {
		rng := newRNG(seed)
		ex := makeExWithFormBody("POST", "/api", "q=search", "field=value")
		origQuery := ex.Request.Query
		origBody := ex.Request.BodyB64
		result := m.Mutate(ex, rng, 1.0)
		if result.Exchange.Request.Query != origQuery {
			queryHit = true
		}
		if result.Exchange.Request.BodyB64 != origBody {
			formHit = true
		}
		if queryHit && formHit {
			break
		}
	}
	if !queryHit {
		t.Error("expected query params to be mutated at least once across 200 seeds")
	}
	if !formHit {
		t.Error("expected form body to be mutated at least once across 200 seeds")
	}
}

func TestParamMutator_AllSeeds(t *testing.T) {
	m := &ParamMutator{}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api", "page=1&limit=10")
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Errorf("seed %d: expected at least one operator", seed)
		}
	}
}

func TestPipeline_ParamsMutator(t *testing.T) {
	cfg := Config{
		Params:    true,
		Intensity: 1.0,
		MaxURLLen: 8192,
	}
	p := NewPipeline(cfg)
	paramHit := false
	for seed := int64(0); seed < 100; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api", "key=value")
		result := p.Mutate(ex, rng, 1.0)
		for _, op := range result.Operators {
			if op == "param:string_mutation" {
				paramHit = true
				break
			}
		}
		if paramHit {
			break
		}
	}
	if !paramHit {
		t.Error("expected param:string_mutation to be applied at least once across 100 seeds")
	}
}

func TestFuzzStrings_Shared(t *testing.T) {
	if len(fuzzStrings) != 77 {
		t.Errorf("expected 77 fuzz strings, got %d", len(fuzzStrings))
	}
	// Spot-check known payloads across all categories
	found := map[string]bool{
		// Original
		"<script>alert(1)</script>": false,
		"' OR '1'='1":               false,
		"../../../etc/passwd":       false,
		// XXE
		`<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><foo>&xxe;</foo>`: false,
		// SSRF
		"http://169.254.169.254/latest/meta-data/": false,
		// Command injection
		"`id`":      false,
		"$(whoami)": false,
		// Prototype pollution
		"__proto__":   false,
		"constructor": false,
	}
	for _, s := range fuzzStrings {
		if _, ok := found[s]; ok {
			found[s] = true
		}
	}
	for payload, present := range found {
		if !present {
			t.Errorf("expected fuzz string %q not found", payload)
		}
	}
}

// --- Operator filter tests ---

func TestFilterOperators_Empty(t *testing.T) {
	result := filterOperators(nil, allURIOps)
	if len(result) != len(allURIOps) {
		t.Fatalf("expected %d ops for nil enabled, got %d", len(allURIOps), len(result))
	}
	result = filterOperators([]string{}, allURIOps)
	if len(result) != len(allURIOps) {
		t.Fatalf("expected %d ops for empty enabled, got %d", len(allURIOps), len(result))
	}
}

func TestFilterOperators_Subset(t *testing.T) {
	result := filterOperators([]string{"path_segment", "query_param"}, allURIOps)
	if len(result) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(result))
	}
	if result[0] != "path_segment" || result[1] != "query_param" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestFilterOperators_All(t *testing.T) {
	result := filterOperators(allURIOps, allURIOps)
	if len(result) != len(allURIOps) {
		t.Fatalf("expected %d ops, got %d", len(allURIOps), len(result))
	}
}

func TestFilterOperators_UnknownFallsBack(t *testing.T) {
	result := filterOperators([]string{"nonexistent"}, allURIOps)
	if len(result) != len(allURIOps) {
		t.Fatalf("expected fallback to all ops for unknown, got %d", len(result))
	}
}

func TestFilterOperators_AllPrimitiveOps(t *testing.T) {
	result := filterOperators(nil, allPrimitiveOps)
	if len(result) != 6 {
		t.Fatalf("expected 6 primitive ops, got %d", len(result))
	}
}

func TestFilterOperators_AllSeqOps(t *testing.T) {
	result := filterOperators(nil, AllSeqOps)
	if len(result) != 4 {
		t.Fatalf("expected 4 seq ops, got %d", len(result))
	}
}

func TestURIMutator_OperatorFilter(t *testing.T) {
	// Only enable path_segment and query_param
	m := &URIMutator{MaxURLLen: 8192, EnabledOps: []string{"path_segment", "query_param"}}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api/users/123", "page=1&limit=10")
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected at least one operator")
		}
		op := result.Operators[0]
		if op != "uri:path_segment" && op != "uri:query_param" {
			t.Errorf("unexpected operator %q (expected path_segment or query_param only)", op)
		}
	}
}

func TestHeaderMutator_OperatorFilter(t *testing.T) {
	m := &HeaderMutator{MaxHdrLen: 8192, EnabledOps: []string{"add"}}
	for seed := int64(0); seed < 30; seed++ {
		rng := newRNG(seed)
		ex := makeEx("GET", "/api", "")
		result := m.Mutate(ex, rng, 1.0)
		if result.Operators[0] != "header:add" {
			t.Errorf("expected header:add, got %v", result.Operators)
		}
	}
}

func TestJSONMutator_OperatorFilter(t *testing.T) {
	m := &JSONMutator{MaxBodyLen: 1 << 20, EnabledOps: []string{"string_mutation"}}
	body := map[string]interface{}{"name": "test", "age": 42}
	for seed := int64(0); seed < 30; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api", body)
		result := m.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected operator")
		}
		if result.Operators[0] != "json:string_mutation" {
			t.Errorf("expected json:string_mutation, got %v", result.Operators)
		}
	}
}

func TestPrimitiveMutator_OperatorFilter(t *testing.T) {
	data := make([]byte, 100)
	for i := range data {
		data[i] = byte(i)
	}
	ex := model.Exchange{
		Request: model.RequestData{
			BodyB64: base64.StdEncoding.EncodeToString(data),
		},
	}
	m := &PrimitiveMutator{EnabledOps: []string{"bitflip"}}
	for seed := int64(0); seed < 30; seed++ {
		rng := newRNG(seed)
		result := m.Mutate(ex, rng, 1.0)
		if result.Operators[0] != "primitive:bitflip" {
			t.Errorf("expected primitive:bitflip, got %v", result.Operators)
		}
	}
}

func TestSeqMutator_OperatorFilter(t *testing.T) {
	m := &SeqMutator{EnabledOps: []string{"swap"}}
	exs := []model.Exchange{
		makeEx("GET", "/a", ""),
		makeEx("POST", "/b", ""),
		makeEx("PUT", "/c", ""),
	}
	for seed := int64(0); seed < 30; seed++ {
		rng := newRNG(seed)
		result := m.Mutate(exs, rng, 0.5)
		if result.Operators[0] != "seq:swap" {
			t.Errorf("expected seq:swap, got %v", result.Operators)
		}
	}
}

func TestPipeline_OperatorFiltering(t *testing.T) {
	cfg := Config{
		PathQuery:      true,
		JSONBody:       true,
		Intensity:      1.0,
		MaxURLLen:      8192,
		MaxBodyLen:     1 << 20,
		URIEnabledOps:  []string{"path_segment"},
		JSONEnabledOps: []string{"string_mutation"},
	}
	p := NewPipeline(cfg)
	body := map[string]interface{}{"key": "value"}
	for seed := int64(0); seed < 50; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api/test", body)
		result := p.Mutate(ex, rng, 1.0)
		if len(result.Operators) == 0 {
			t.Error("expected at least one operator")
		}
		for _, op := range result.Operators {
			if op != "uri:path_segment" && op != "json:string_mutation" && !strings.HasPrefix(op, "primitive:") {
				t.Errorf("unexpected operator %q", op)
			}
		}
	}
}

func TestPipeline_BackwardCompatibleNoOperatorConfig(t *testing.T) {
	// No operator lists = all operators enabled (backward compatible)
	cfg := Config{
		PathQuery:  true,
		Headers:    true,
		JSONBody:   true,
		Intensity:  1.0,
		MaxURLLen:  8192,
		MaxHdrLen:  8192,
		MaxBodyLen: 1 << 20,
	}
	p := NewPipeline(cfg)
	body := map[string]interface{}{"key": "value"}
	foundURI := false
	foundHeader := false
	foundJSON := false
	// With enough seeds, we should see operators from all categories
	for seed := int64(0); seed < 200; seed++ {
		rng := newRNG(seed)
		ex := makeExWithBody("POST", "/api/test", body)
		result := p.Mutate(ex, rng, 1.0)
		for _, op := range result.Operators {
			if strings.HasPrefix(op, "uri:") {
				foundURI = true
			}
			if strings.HasPrefix(op, "header:") {
				foundHeader = true
			}
			if strings.HasPrefix(op, "json:") {
				foundJSON = true
			}
		}
		if foundURI && foundHeader && foundJSON {
			break
		}
	}
	if !foundURI {
		t.Error("expected URI operators to appear")
	}
	if !foundHeader {
		t.Error("expected Header operators to appear")
	}
	if !foundJSON {
		t.Error("expected JSON operators to appear")
	}
}
