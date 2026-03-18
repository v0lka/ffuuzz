package replayer

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

func TestNewWorkerContext(t *testing.T) {
	wc := NewWorkerContext(5*time.Second, zerolog.Nop())
	if wc == nil {
		t.Fatal("expected non-nil WorkerContext")
	}
	if wc.CookieJar == nil {
		t.Error("expected non-nil CookieJar")
	}
	if wc.Variables == nil {
		t.Error("expected non-nil Variables")
	}
	if wc.Client == nil {
		t.Error("expected non-nil Client")
	}
	if wc.Client.Timeout != 5*time.Second {
		t.Errorf("Client.Timeout = %v, want 5s", wc.Client.Timeout)
	}
}

func TestNewWorkerContext_ZeroTimeout(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	if wc.Client.Timeout != 0 {
		t.Errorf("Client.Timeout = %v, want 0", wc.Client.Timeout)
	}
}

func TestApplySubstitutions_NoVars(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	ex := model.Exchange{
		Request: model.RequestData{
			Path:    "/api/test",
			Query:   "key=val",
			BodyB64: "body",
		},
	}
	wc.ApplySubstitutions(&ex)
	if ex.Request.Path != "/api/test" {
		t.Errorf("Path = %q", ex.Request.Path)
	}
}

func TestApplySubstitutions_WithVars(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	wc.Variables["token"] = "abc123"
	wc.Variables["id"] = "42"

	ex := model.Exchange{
		Request: model.RequestData{
			Path:    "/api/users/{{id}}",
			Query:   "token={{token}}",
			BodyB64: "data-{{token}}",
			Headers: map[string][]string{
				"Authorization": {"Bearer {{token}}"},
			},
		},
	}
	wc.ApplySubstitutions(&ex)

	if ex.Request.Path != "/api/users/42" {
		t.Errorf("Path = %q, want /api/users/42", ex.Request.Path)
	}
	if ex.Request.Query != "token=abc123" {
		t.Errorf("Query = %q", ex.Request.Query)
	}
	if ex.Request.BodyB64 != "data-abc123" {
		t.Errorf("BodyB64 = %q", ex.Request.BodyB64)
	}
	if ex.Request.Headers["Authorization"][0] != "Bearer abc123" {
		t.Errorf("Header = %q", ex.Request.Headers["Authorization"][0])
	}
}

func TestApplySubstitutions_NilHeaders(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	wc.Variables["x"] = "y"
	ex := model.Exchange{
		Request: model.RequestData{
			Path:    "/{{x}}",
			Headers: nil,
		},
	}
	wc.ApplySubstitutions(&ex)
	if ex.Request.Path != "/y" {
		t.Errorf("Path = %q", ex.Request.Path)
	}
}

func TestExtractVariables_FromHeader(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	resp := &http.Response{
		Header: http.Header{
			"X-Token": {"abc-def-123"},
		},
	}
	rules := []ExtractionRule{
		{Name: "token", Source: "header", Header: "X-Token", Regex: `^(.+)$`},
	}
	wc.ExtractVariables(resp, nil, rules)
	if wc.Variables["token"] != "abc-def-123" {
		t.Errorf("token = %q, want abc-def-123", wc.Variables["token"])
	}
}

func TestExtractVariables_FromBody(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	resp := &http.Response{Header: http.Header{}}
	body := []byte(`{"id":"user-42","name":"test"}`)
	rules := []ExtractionRule{
		{Name: "user_id", Source: "body", Regex: `"id":"([^"]+)"`},
	}
	wc.ExtractVariables(resp, body, rules)
	if wc.Variables["user_id"] != "user-42" {
		t.Errorf("user_id = %q, want user-42", wc.Variables["user_id"])
	}
}

func TestExtractVariables_NoMatch(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	resp := &http.Response{Header: http.Header{}}
	rules := []ExtractionRule{
		{Name: "missing", Source: "body", Regex: `not-found-(\d+)`},
	}
	wc.ExtractVariables(resp, []byte("no match here"), rules)
	if _, ok := wc.Variables["missing"]; ok {
		t.Error("expected no variable extracted")
	}
}

func TestExtractVariables_InvalidRegex(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	resp := &http.Response{Header: http.Header{}}
	rules := []ExtractionRule{
		{Name: "bad", Source: "body", Regex: `[invalid`},
	}
	wc.ExtractVariables(resp, []byte("anything"), rules)
	if _, ok := wc.Variables["bad"]; ok {
		t.Error("expected no variable for invalid regex")
	}
}

func TestExtractVariables_EmptyRegex(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	resp := &http.Response{Header: http.Header{}}
	rules := []ExtractionRule{
		{Name: "empty", Source: "body", Regex: ""},
	}
	wc.ExtractVariables(resp, []byte("data"), rules)
	if _, ok := wc.Variables["empty"]; ok {
		t.Error("expected no variable for empty regex")
	}
}

func TestExtractVariables_UnknownSource(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	resp := &http.Response{Header: http.Header{}}
	rules := []ExtractionRule{
		{Name: "x", Source: "unknown", Regex: `(.+)`},
	}
	wc.ExtractVariables(resp, []byte("data"), rules)
	if _, ok := wc.Variables["x"]; ok {
		t.Error("expected no variable for unknown source")
	}
}

func TestUpdateCookies(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	reqURL, _ := url.Parse("http://example.com/api")
	resp := &http.Response{
		Header: http.Header{
			"Set-Cookie": {"session=abc; Path=/"},
		},
	}
	wc.UpdateCookies(resp, reqURL)
	cookies := wc.CookieJar.Cookies(reqURL)
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}
	if cookies[0].Name != "session" || cookies[0].Value != "abc" {
		t.Errorf("cookie = %v", cookies[0])
	}
}

func TestUpdateCookies_NilResp(t *testing.T) {
	wc := NewWorkerContext(0, zerolog.Nop())
	reqURL, _ := url.Parse("http://example.com")
	// Should not panic
	wc.UpdateCookies(nil, reqURL)
}

func TestUpdateCookies_NilJar(t *testing.T) {
	wc := &WorkerContext{}
	reqURL, _ := url.Parse("http://example.com")
	resp := &http.Response{Header: http.Header{}}
	// Should not panic
	wc.UpdateCookies(resp, reqURL)
}
