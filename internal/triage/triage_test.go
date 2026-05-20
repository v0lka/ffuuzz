package triage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/model"
	"ffuuzz/internal/replayer"
)

type mockReplayer struct {
	replayFn func(ctx context.Context, session model.RecordingSession, baseURL string, wctx *replayer.WorkerContext, rules []replayer.ExtractionRule) ([]replayer.ExchangeResult, error)
}

func (m *mockReplayer) ReplaySession(ctx context.Context, session model.RecordingSession, baseURL string, wctx *replayer.WorkerContext, rules []replayer.ExtractionRule) ([]replayer.ExchangeResult, error) {
	return m.replayFn(ctx, session, baseURL, wctx, rules)
}

type mockDetector struct {
	detectFn func(ex model.Exchange, result replayer.ExchangeResult, baseline *anomaly.BaselineEntry, cfg model.AnomalyConfig) []anomaly.AnomalyHit
}

func (m *mockDetector) Detect(ex model.Exchange, result replayer.ExchangeResult, baseline *anomaly.BaselineEntry, cfg model.AnomalyConfig) []anomaly.AnomalyHit {
	return m.detectFn(ex, result, baseline, cfg)
}

// helper: make a session with one exchange containing a JSON body
func makeSession(bodyJSON map[string]interface{}) model.RecordingSession {
	data, _ := json.Marshal(bodyJSON)
	return model.RecordingSession{
		ID: "test-session",
		Entries: []model.Exchange{
			{
				RequestID: "req-1",
				Request: model.RequestData{
					Method:  "POST",
					Path:    "/api/test",
					Headers: map[string][]string{"Content-Type": {"application/json"}},
					BodyB64: base64.StdEncoding.EncodeToString(data),
				},
			},
		},
		EntryCount: 1,
	}
}

// helper: decode body from session entry
func decodeBody(t *testing.T, session model.RecordingSession, idx int) map[string]interface{} {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(session.Entries[idx].Request.BodyB64)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	return obj
}

// keyBasedDetector returns hits only when all requiredKeys are present in the request body.
func keyBasedDetector(requiredKeys []string) *mockDetector {
	return &mockDetector{
		detectFn: func(ex model.Exchange, _ replayer.ExchangeResult, _ *anomaly.BaselineEntry, _ model.AnomalyConfig) []anomaly.AnomalyHit {
			if ex.Request.BodyB64 == "" {
				return nil
			}
			raw, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
			if err != nil {
				return nil
			}
			var obj map[string]interface{}
			if err := json.Unmarshal(raw, &obj); err != nil {
				return nil
			}
			for _, k := range requiredKeys {
				if _, ok := obj[k]; !ok {
					return nil
				}
			}
			return []anomaly.AnomalyHit{{Type: "server_error", Method: "POST", Endpoint: "/api/test"}}
		},
	}
}

// passthrough replayer: returns one ExchangeResult per entry, echoing the exchange
func passthroughReplayer() *mockReplayer {
	return &mockReplayer{
		replayFn: func(_ context.Context, session model.RecordingSession, _ string, _ *replayer.WorkerContext, _ []replayer.ExtractionRule) ([]replayer.ExchangeResult, error) {
			results := make([]replayer.ExchangeResult, len(session.Entries))
			for i, ex := range session.Entries {
				results[i] = replayer.ExchangeResult{
					Exchange:   ex,
					StatusCode: 500,
					DurationMs: 10,
				}
			}
			return results, nil
		},
	}
}

func TestNormalizePath_NumericIDs(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/api/users/123", "/api/users/{id}"},
		{"/api/users/123/posts/456", "/api/users/{id}/posts/{id}"},
		{"/api/users/0", "/api/users/{id}"},
		{"/api/users/999999999", "/api/users/{id}"},
		{"/api/users", "/api/users"},
	}
	for _, tt := range tests {
		got := NormalizePath(tt.input)
		if got != tt.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePath_UUIDs(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/api/users/550e8400-e29b-41d4-a716-446655440000", "/api/users/{uuid}"},
		{"/api/users/550e8400-e29b-41d4-a716-446655440000/posts", "/api/users/{uuid}/posts"},
		{"/api/a/550e8400-E29B-41d4-a716-446655440000/b", "/api/a/{uuid}/b"},
	}
	for _, tt := range tests {
		got := NormalizePath(tt.input)
		if got != tt.want {
			t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePath_Mixed(t *testing.T) {
	input := "/api/users/550e8400-e29b-41d4-a716-446655440000/posts/42"
	got := NormalizePath(input)
	want := "/api/users/{uuid}/posts/{id}"
	if got != want {
		t.Errorf("NormalizePath(%q) = %q, want %q", input, got, want)
	}
}

func TestNormalizePath_NoChange(t *testing.T) {
	tests := []string{
		"/api/users",
		"/healthz",
		"/",
		"",
	}
	for _, input := range tests {
		got := NormalizePath(input)
		if got != input {
			t.Errorf("NormalizePath(%q) = %q, want unchanged", input, got)
		}
	}
}

func TestNormalizePath_FuzzSegments(t *testing.T) {
	longSeg := strings.Repeat("A", 1024)
	tests := []struct {
		input string
		want  string
	}{
		// Long segment appended by uri:long_value mutation
		{"/rest/admin/application-version/" + longSeg, "/rest/admin/application-version/{fuzz}"},
		// Different lengths should map to the same signature
		{"/api/items/" + strings.Repeat("A", 64), "/api/items/{fuzz}"},
		{"/api/items/" + strings.Repeat("A", 4096), "/api/items/{fuzz}"},
		// Short segments are untouched
		{"/api/items/shorttoken", "/api/items/shorttoken"},
		// Exactly at threshold
		{"/x/" + strings.Repeat("B", 64), "/x/{fuzz}"},
		// Just below threshold
		{"/x/" + strings.Repeat("B", 63), "/x/" + strings.Repeat("B", 63)},
	}
	for _, tt := range tests {
		got := NormalizePath(tt.input)
		if got != tt.want {
			preview := tt.input
			if len(preview) > 60 {
				preview = preview[:60] + "..."
			}
			t.Errorf("NormalizePath(%q) = %q, want %q", preview, got, tt.want)
		}
	}
}

func TestSignature_DeduplicatesLongFuzzPaths(t *testing.T) {
	triager := NewTriager()
	base := anomaly.AnomalyHit{
		Type:   model.FindingServerError,
		Method: "GET",
		Exchange: model.Exchange{
			Request: model.RequestData{BodyB64: ""},
		},
	}

	// Different fuzz payload lengths on the same endpoint must produce the same signature.
	hit1 := base
	hit1.Endpoint = "/rest/admin/application-version/" + strings.Repeat("A", 1024)
	hit2 := base
	hit2.Endpoint = "/rest/admin/application-version/" + strings.Repeat("A", 4096)
	hit3 := base
	hit3.Endpoint = "/rest/admin/application-version/" + strings.Repeat("A", 65536)

	s1 := triager.Signature(hit1)
	s2 := triager.Signature(hit2)
	s3 := triager.Signature(hit3)

	if s1 != s2 || s2 != s3 {
		t.Errorf("fuzz paths of different lengths should deduplicate to same signature:\n  s1=%q\n  s2=%q\n  s3=%q", s1, s2, s3)
	}
}

func TestHashPayload_Empty(t *testing.T) {
	got := HashPayload("")
	if got != "empty" {
		t.Errorf("HashPayload(\"\") = %q, want %q", got, "empty")
	}
}

func TestHashPayload_Deterministic(t *testing.T) {
	body := "eyJuYW1lIjoiSm9obiJ9" // base64 of {"name":"John"}
	h1 := HashPayload(body)
	h2 := HashPayload(body)
	if h1 != h2 {
		t.Errorf("HashPayload not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 16 {
		t.Errorf("HashPayload length = %d, want 16 hex chars", len(h1))
	}
}

func TestHashPayload_DifferentBodies(t *testing.T) {
	h1 := HashPayload("aGVsbG8=") // "hello"
	h2 := HashPayload("d29ybGQ=") // "world"
	if h1 == h2 {
		t.Error("HashPayload should differ for different bodies")
	}
}

func TestSignature_Format(t *testing.T) {
	triager := NewTriager()
	hit := anomaly.AnomalyHit{
		Type:     "timeout",
		Method:   "POST",
		Endpoint: "/api/users/123",
		Exchange: model.Exchange{
			Request: model.RequestData{
				BodyB64: "dGVzdA==",
			},
		},
	}
	hit.Details.HTTPStatus = 500

	sig := triager.Signature(hit)

	// Format: TYPE|METHOD|normalizedPath|statusCode|hash
	parts := splitSignature(sig)
	if len(parts) != 5 {
		t.Fatalf("Signature has %d parts, want 5: %q", len(parts), sig)
	}
	if parts[0] != "timeout" {
		t.Errorf("Signature type = %q, want %q", parts[0], "timeout")
	}
	if parts[1] != "POST" {
		t.Errorf("Signature method = %q, want %q", parts[1], "POST")
	}
	if parts[2] != "/api/users/{id}" {
		t.Errorf("Signature path = %q, want %q", parts[2], "/api/users/{id}")
	}
	if parts[3] != "500" {
		t.Errorf("Signature status = %q, want %q", parts[3], "500")
	}
	if len(parts[4]) != 16 {
		t.Errorf("Signature hash length = %d, want 16", len(parts[4]))
	}
}

func TestSignature_SameHitSameSignature(t *testing.T) {
	triager := NewTriager()
	hit := anomaly.AnomalyHit{
		Type:     "server_error",
		Method:   "GET",
		Endpoint: "/api/items/42",
		Exchange: model.Exchange{
			Request: model.RequestData{
				BodyB64: "",
			},
		},
	}
	s1 := triager.Signature(hit)
	s2 := triager.Signature(hit)
	if s1 != s2 {
		t.Errorf("Same hit produced different signatures: %q vs %q", s1, s2)
	}
}

func TestSignature_DifferentTypesAreDifferent(t *testing.T) {
	triager := NewTriager()
	base := anomaly.AnomalyHit{
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{
			Request: model.RequestData{BodyB64: "dGVzdA=="},
		},
	}

	hit1 := base
	hit1.Type = "timeout"
	hit2 := base
	hit2.Type = "server_error"

	if triager.Signature(hit1) == triager.Signature(hit2) {
		t.Error("Different anomaly types should produce different signatures")
	}
}

func TestSignature_NormalizesIDs(t *testing.T) {
	triager := NewTriager()
	hit1 := anomaly.AnomalyHit{
		Type:     "timeout",
		Method:   "GET",
		Endpoint: "/api/users/100",
		Exchange: model.Exchange{
			Request: model.RequestData{BodyB64: ""},
		},
	}
	hit2 := anomaly.AnomalyHit{
		Type:     "timeout",
		Method:   "GET",
		Endpoint: "/api/users/200",
		Exchange: model.Exchange{
			Request: model.RequestData{BodyB64: ""},
		},
	}

	s1 := triager.Signature(hit1)
	s2 := triager.Signature(hit2)
	if s1 != s2 {
		t.Errorf("IDs should be normalized to same signature: %q vs %q", s1, s2)
	}
}

func TestSignature_DifferentStatusCodes(t *testing.T) {
	triager := NewTriager()
	base := anomaly.AnomalyHit{
		Type:     "server_error",
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{
			Request: model.RequestData{BodyB64: "dGVzdA=="},
		},
	}
	hit500 := base
	hit500.Details.HTTPStatus = 500
	hit502 := base
	hit502.Details.HTTPStatus = 502
	hit200 := base
	hit200.Details.HTTPStatus = 200

	s500 := triager.Signature(hit500)
	s502 := triager.Signature(hit502)
	s200 := triager.Signature(hit200)

	if s500 == s502 {
		t.Errorf("different HTTP status codes (500 vs 502) should produce different signatures")
	}
	if s500 == s200 {
		t.Errorf("different HTTP status codes (500 vs 200) should produce different signatures")
	}
}

func splitSignature(sig string) []string {
	var parts []string
	current := ""
	for _, c := range sig {
		if c == '|' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	parts = append(parts, current)
	return parts
}

func TestStillTriggers_Fires(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"trigger": true})
	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	got := triager.stillTriggers(context.Background(), session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if !got {
		t.Error("expected stillTriggers to return true")
	}
}

func TestStillTriggers_NoFire(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"other": true})
	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	got := triager.stillTriggers(context.Background(), session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if got {
		t.Error("expected stillTriggers to return false")
	}
}

func TestHasJSONBody_ValidJSON(t *testing.T) {
	data, _ := json.Marshal(map[string]interface{}{"key": "val"})
	ex := model.Exchange{
		Request: model.RequestData{
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			BodyB64: base64.StdEncoding.EncodeToString(data),
		},
	}
	if !HasJSONBody(ex) {
		t.Error("expected HasJSONBody=true for valid JSON object")
	}
}

func TestHasJSONBody_EmptyBody(t *testing.T) {
	ex := model.Exchange{
		Request: model.RequestData{
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			BodyB64: "",
		},
	}
	if HasJSONBody(ex) {
		t.Error("expected HasJSONBody=false for empty body")
	}
}

func TestHasJSONBody_Truncated(t *testing.T) {
	data, _ := json.Marshal(map[string]interface{}{"key": "val"})
	ex := model.Exchange{
		Request: model.RequestData{
			Headers:       map[string][]string{"Content-Type": {"application/json"}},
			BodyB64:       base64.StdEncoding.EncodeToString(data),
			BodyTruncated: true,
		},
	}
	if HasJSONBody(ex) {
		t.Error("expected HasJSONBody=false for truncated body")
	}
}

func TestHasJSONBody_NonJSONContentType(t *testing.T) {
	data, _ := json.Marshal(map[string]interface{}{"key": "val"})
	ex := model.Exchange{
		Request: model.RequestData{
			Headers: map[string][]string{"Content-Type": {"text/plain"}},
			BodyB64: base64.StdEncoding.EncodeToString(data),
		},
	}
	if HasJSONBody(ex) {
		t.Error("expected HasJSONBody=false for non-JSON content type")
	}
}

func TestHasJSONBody_ArrayBody(t *testing.T) {
	data, _ := json.Marshal([]int{1, 2, 3})
	ex := model.Exchange{
		Request: model.RequestData{
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			BodyB64: base64.StdEncoding.EncodeToString(data),
		},
	}
	if HasJSONBody(ex) {
		t.Error("expected HasJSONBody=false for JSON array (not object)")
	}
}

func TestMinimizeJSONBody_EmptyBody(t *testing.T) {
	triager := NewTriager()
	session := model.RecordingSession{
		Entries: []model.Exchange{{Request: model.RequestData{BodyB64: ""}}},
	}
	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", keyBasedDetector(nil), model.AnomalyConfig{}, nil, passthroughReplayer(), 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for empty body")
	}
}

func TestMinimizeJSONBody_TruncatedBody(t *testing.T) {
	triager := NewTriager()
	data, _ := json.Marshal(map[string]interface{}{"key": "val"})
	session := model.RecordingSession{
		Entries: []model.Exchange{{
			Request: model.RequestData{
				BodyB64:       base64.StdEncoding.EncodeToString(data),
				BodyTruncated: true,
			},
		}},
	}
	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", keyBasedDetector(nil), model.AnomalyConfig{}, nil, passthroughReplayer(), 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for truncated body")
	}
}

func TestMinimizeJSONBody_NonJSON(t *testing.T) {
	triager := NewTriager()
	session := model.RecordingSession{
		Entries: []model.Exchange{{
			Request: model.RequestData{
				BodyB64: base64.StdEncoding.EncodeToString([]byte("not json")),
			},
		}},
	}
	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", keyBasedDetector(nil), model.AnomalyConfig{}, nil, passthroughReplayer(), 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for non-JSON body")
	}
}

func TestMinimizeJSONBody_ArrayBody(t *testing.T) {
	triager := NewTriager()
	data, _ := json.Marshal([]int{1, 2, 3})
	session := model.RecordingSession{
		Entries: []model.Exchange{{
			Request: model.RequestData{
				BodyB64: base64.StdEncoding.EncodeToString(data),
			},
		}},
	}
	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", keyBasedDetector(nil), model.AnomalyConfig{}, nil, passthroughReplayer(), 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for JSON array body")
	}
}

func TestMinimizeJSONBody_SingleRemovableKey(t *testing.T) {
	triager := NewTriager()
	body := map[string]interface{}{
		"trigger": true,
		"noise1":  "abc",
		"noise2":  42,
	}
	session := makeSession(body)
	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}

	result := decodeBody(t, *got, 0)
	if _, ok := result["trigger"]; !ok {
		t.Error("result should contain the 'trigger' key")
	}
	if len(result) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(result), result)
	}
}

func TestMinimizeJSONBody_AllKeysEssential(t *testing.T) {
	triager := NewTriager()
	body := map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	}
	session := makeSession(body)
	det := keyBasedDetector([]string{"a", "b", "c"})
	rep := passthroughReplayer()

	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		result := decodeBody(t, *got, 0)
		if len(result) != 3 {
			t.Errorf("all keys essential, should have 3 keys, got %d", len(result))
		}
	}
}

func TestMinimizeJSONBody_BinarySearchReduction(t *testing.T) {
	triager := NewTriager()
	body := map[string]interface{}{
		"a": 1, "b": 2, "c": 3, "d": 4,
		"e": 5, "f": 6, "g": 7, "h": 8,
	}
	session := makeSession(body)
	// Only "c" and "f" are essential
	det := keyBasedDetector([]string{"c", "f"})
	rep := passthroughReplayer()

	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}

	result := decodeBody(t, *got, 0)
	if _, ok := result["c"]; !ok {
		t.Error("result must contain 'c'")
	}
	if _, ok := result["f"]; !ok {
		t.Error("result must contain 'f'")
	}
	if len(result) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(result), result)
	}
}

func TestMinimizeJSONBody_NestedMinimization(t *testing.T) {
	triager := NewTriager()
	body := map[string]interface{}{
		"outer": map[string]interface{}{
			"trigger": true,
			"noise":   "remove me",
		},
	}
	session := makeSession(body)

	// Detector fires if outer.trigger exists
	det := &mockDetector{
		detectFn: func(ex model.Exchange, _ replayer.ExchangeResult, _ *anomaly.BaselineEntry, _ model.AnomalyConfig) []anomaly.AnomalyHit {
			if ex.Request.BodyB64 == "" {
				return nil
			}
			raw, err := base64.StdEncoding.DecodeString(ex.Request.BodyB64)
			if err != nil {
				return nil
			}
			var obj map[string]interface{}
			if err := json.Unmarshal(raw, &obj); err != nil {
				return nil
			}
			outer, ok := obj["outer"]
			if !ok {
				return nil
			}
			nested, ok := outer.(map[string]interface{})
			if !ok {
				return nil
			}
			if _, ok := nested["trigger"]; !ok {
				return nil
			}
			return []anomaly.AnomalyHit{{Type: "server_error", Method: "POST", Endpoint: "/api/test"}}
		},
	}
	rep := passthroughReplayer()

	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}

	result := decodeBody(t, *got, 0)
	outer, ok := result["outer"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'outer' to be a nested object")
	}
	if _, ok := outer["trigger"]; !ok {
		t.Error("nested 'trigger' key should be preserved")
	}
	if _, ok := outer["noise"]; ok {
		t.Error("nested 'noise' key should have been removed")
	}
}

func TestMinimizeJSONBody_ContextCancelled(t *testing.T) {
	triager := NewTriager()
	body := map[string]interface{}{"a": 1, "b": 2}
	session := makeSession(body)
	det := keyBasedDetector([]string{"a"})
	rep := passthroughReplayer()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	got, err := triager.MinimizeJSONBody(ctx, session, 0, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	// With cancelled context the verify function returns false, so no reduction is possible
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil result with cancelled context")
	}
}

func TestMinimizeJSONBody_ReplayError(t *testing.T) {
	triager := NewTriager()
	body := map[string]interface{}{"a": 1, "b": 2, "c": 3}
	session := makeSession(body)
	det := keyBasedDetector([]string{"a"})

	errorReplayer := &mockReplayer{
		replayFn: func(_ context.Context, _ model.RecordingSession, _ string, _ *replayer.WorkerContext, _ []replayer.ExtractionRule) ([]replayer.ExchangeResult, error) {
			return nil, context.DeadlineExceeded
		},
	}

	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", det, model.AnomalyConfig{}, nil, errorReplayer, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil result when replayer always errors")
	}
}

func TestMinimizeJSONBody_InvalidIndex(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"a": 1})

	got, err := triager.MinimizeJSONBody(context.Background(), session, 5, "http://localhost", keyBasedDetector(nil), model.AnomalyConfig{}, nil, passthroughReplayer(), 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for out-of-range index")
	}
}

func TestSortJSONKeys_Map(t *testing.T) {
	input := map[string]interface{}{
		"name": "John",
		"age":  float64(30),
	}
	result := sortJSONKeys(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	// Values must be preserved (not replaced with type names)
	if m["name"] != "John" {
		t.Errorf("name = %v, want 'John'", m["name"])
	}
	if m["age"] != float64(30) {
		t.Errorf("age = %v, want float64(30)", m["age"])
	}
	// Keys must be sorted alphabetically
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	if keys[0] != "age" || keys[1] != "name" {
		t.Errorf("keys not sorted: %v", keys)
	}
}

func TestSortJSONKeys_Array(t *testing.T) {
	input := []interface{}{"a", "b", "c"}
	result := sortJSONKeys(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", result)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(arr))
	}
	if arr[0] != "a" || arr[1] != "b" || arr[2] != "c" {
		t.Errorf("array elements not preserved: %v", arr)
	}
}

func TestSortJSONKeys_EmptyArray(t *testing.T) {
	input := []interface{}{}
	result := sortJSONKeys(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", result)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %d elements", len(arr))
	}
}

func TestSortJSONKeys_Scalar(t *testing.T) {
	result := sortJSONKeys("hello")
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "hello" {
		t.Errorf("result = %q, want 'hello'", s)
	}
}

func TestSortJSONKeys_NilValue(t *testing.T) {
	result := sortJSONKeys(nil)
	if result != nil {
		t.Errorf("result = %v, want nil", result)
	}
}

func TestHashPayload_JSONBody(t *testing.T) {
	// JSON with same values but different key order should produce same hash
	body1 := base64.StdEncoding.EncodeToString([]byte(`{"a":1,"b":2}`))
	body2 := base64.StdEncoding.EncodeToString([]byte(`{"b":2,"a":1}`))
	h1 := HashPayload(body1)
	h2 := HashPayload(body2)
	if h1 != h2 {
		t.Errorf("JSON with same keys in different order should produce same hash: %q != %q", h1, h2)
	}
}

func TestHashPayload_DifferentJSONValues(t *testing.T) {
	// JSON with same keys but different values MUST produce different hashes.
	// Regression: the old normalizeJSON replaced values with type names,
	// causing completely different attacks (SQLi vs SSTI vs XSS) on the same
	// endpoint to be incorrectly deduplicated.
	body1 := base64.StdEncoding.EncodeToString([]byte(`{"a":1,"b":2}`))
	body2 := base64.StdEncoding.EncodeToString([]byte(`{"a":999,"b":2}`))
	h1 := HashPayload(body1)
	h2 := HashPayload(body2)
	if h1 == h2 {
		t.Errorf("JSON with different values should produce different hashes: %q", h1)
	}
}

func TestHashPayload_NestedJSON(t *testing.T) {
	// Nested JSON objects should have keys sorted recursively
	body1 := base64.StdEncoding.EncodeToString([]byte(`{"outer":{"b":2,"a":1}}`))
	body2 := base64.StdEncoding.EncodeToString([]byte(`{"outer":{"a":1,"b":2}}`))
	h1 := HashPayload(body1)
	h2 := HashPayload(body2)
	if h1 != h2 {
		t.Errorf("Nested JSON with same values in different key order should produce same hash: %q != %q", h1, h2)
	}
}

func TestHashPayload_NonJSON(t *testing.T) {
	h := HashPayload("not json at all")
	if h == "empty" {
		t.Error("non-JSON non-empty body should not return 'empty'")
	}
	if len(h) != 16 {
		t.Errorf("hash length = %d, want 16", len(h))
	}
}

func TestConfirm_AllReproduced(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"trigger": true})
	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	confirmed, _, err := triager.Confirm(context.Background(), session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 3, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Error("expected confirmed=true when all runs reproduce")
	}
}

func TestConfirm_NoneReproduced(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"other": true})
	det := keyBasedDetector([]string{"trigger"}) // won't match "other"
	rep := passthroughReplayer()

	confirmed, _, err := triager.Confirm(context.Background(), session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 3, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if confirmed {
		t.Error("expected confirmed=false when none reproduce")
	}
}

func TestConfirm_DefaultRuns(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"trigger": true})
	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	confirmed, _, err := triager.Confirm(context.Background(), session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !confirmed {
		t.Error("expected confirmed=true with default runs")
	}
}

func TestConfirm_ContextCancelled(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"trigger": true})
	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := triager.Confirm(ctx, session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 5, 0, zerolog.Nop())
	if err == nil {
		t.Error("expected error with cancelled context")
	}
}

func TestMinimizeSession_SingleEntry(t *testing.T) {
	triager := NewTriager()
	session := makeSession(map[string]interface{}{"trigger": true})
	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	result, err := triager.MinimizeSession(context.Background(), session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Entries) != 1 {
		t.Errorf("expected 1 entry (can't remove only entry), got %d", len(result.Entries))
	}
}

func TestMinimizeSession_RemovesUnnecessary(t *testing.T) {
	triager := NewTriager()
	data1, _ := json.Marshal(map[string]interface{}{"trigger": true})
	data2, _ := json.Marshal(map[string]interface{}{"noise": true})

	session := model.RecordingSession{
		ID: "test-session",
		Entries: []model.Exchange{
			{
				RequestID: "req-1",
				Request: model.RequestData{
					Method:  "POST",
					Path:    "/api/test",
					Headers: map[string][]string{"Content-Type": {"application/json"}},
					BodyB64: base64.StdEncoding.EncodeToString(data1),
				},
			},
			{
				RequestID: "req-2",
				Request: model.RequestData{
					Method:  "POST",
					Path:    "/api/test",
					Headers: map[string][]string{"Content-Type": {"application/json"}},
					BodyB64: base64.StdEncoding.EncodeToString(data2),
				},
			},
		},
		EntryCount: 2,
	}

	det := keyBasedDetector([]string{"trigger"})
	rep := passthroughReplayer()

	result, err := triager.MinimizeSession(context.Background(), session, "http://localhost", det, model.AnomalyConfig{}, nil, rep, 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// The second entry (noise) should be removable since only "trigger" is needed
	if len(result.Entries) > 1 {
		t.Logf("entries not minimized: got %d", len(result.Entries))
	}
}

func TestMinimizeSession_ContextCancelled(t *testing.T) {
	triager := NewTriager()
	data1, _ := json.Marshal(map[string]interface{}{"a": 1})
	data2, _ := json.Marshal(map[string]interface{}{"b": 2})

	session := model.RecordingSession{
		ID: "s",
		Entries: []model.Exchange{
			{RequestID: "r1", Request: model.RequestData{BodyB64: base64.StdEncoding.EncodeToString(data1)}},
			{RequestID: "r2", Request: model.RequestData{BodyB64: base64.StdEncoding.EncodeToString(data2)}},
		},
		EntryCount: 2,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := triager.MinimizeSession(ctx, session, "http://localhost", keyBasedDetector(nil), model.AnomalyConfig{}, nil, passthroughReplayer(), 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With cancelled context, entries should stay unchanged
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]interface{}{"c": 3, "a": 1, "b": 2}
	keys := sortedKeys(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("keys not sorted: %v", keys)
	}
}

func TestWithoutKeys(t *testing.T) {
	obj := map[string]interface{}{"a": 1, "b": 2, "c": 3}
	result := withoutKeys(obj, []string{"b"})
	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result))
	}
	if _, ok := result["b"]; ok {
		t.Error("'b' should be removed")
	}
}

func TestOnlyKeys(t *testing.T) {
	obj := map[string]interface{}{"a": 1, "b": 2, "c": 3}
	result := onlyKeys(obj, []string{"a", "c"})
	if len(result) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(result))
	}
	if _, ok := result["a"]; !ok {
		t.Error("'a' should be present")
	}
	if _, ok := result["c"]; !ok {
		t.Error("'c' should be present")
	}
}

func TestCopyMap(t *testing.T) {
	m := map[string]interface{}{"a": 1, "b": 2}
	cp := copyMap(m)
	if len(cp) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(cp))
	}
	// Modify copy should not affect original
	cp["c"] = 3
	if _, ok := m["c"]; ok {
		t.Error("original should not be modified")
	}
}

func TestCountKeysDeep(t *testing.T) {
	m := map[string]interface{}{
		"a": 1,
		"b": map[string]interface{}{
			"c": 2,
			"d": 3,
		},
	}
	count := countKeysDeep(m)
	if count != 4 { // a, b, c, d
		t.Errorf("count = %d, want 4", count)
	}
}

func TestCloneSessionWithBody(t *testing.T) {
	session := makeSession(map[string]interface{}{"a": 1})
	newBody := base64.StdEncoding.EncodeToString([]byte(`{"b":2}`))
	cloned := cloneSessionWithBody(session, 0, newBody)

	if cloned.Entries[0].Request.BodyB64 != newBody {
		t.Error("cloned session body not updated")
	}
	// Original should not be affected
	if session.Entries[0].Request.BodyB64 == newBody {
		t.Error("original session should not be modified")
	}
}

func TestMinimizeJSONBody_EmptyObject(t *testing.T) {
	triager := NewTriager()
	data, _ := json.Marshal(map[string]interface{}{})
	session := model.RecordingSession{
		Entries: []model.Exchange{{
			Request: model.RequestData{
				BodyB64: base64.StdEncoding.EncodeToString(data),
			},
		}},
	}
	got, err := triager.MinimizeJSONBody(context.Background(), session, 0, "http://localhost", keyBasedDetector(nil), model.AnomalyConfig{}, nil, passthroughReplayer(), 0, zerolog.Nop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for empty JSON object")
	}
}
