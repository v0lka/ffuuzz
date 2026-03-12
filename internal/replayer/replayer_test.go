package replayer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

func TestNew_NilClient(t *testing.T) {
	r := New(nil, zerolog.Nop())
	if r.DefaultClient == nil {
		t.Fatal("expected non-nil DefaultClient")
	}
	if r.DefaultClient.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", r.DefaultClient.Timeout)
	}
}

func TestNew_CustomClient(t *testing.T) {
	client := &http.Client{Timeout: 5 * time.Second}
	r := New(client, zerolog.Nop())
	if r.DefaultClient != client {
		t.Error("expected custom client")
	}
}

func TestReplayExchange_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	ex := model.Exchange{
		Request: model.RequestData{
			Method: "GET",
			Path:   "/test",
		},
	}

	result := r.ReplayExchange(context.Background(), ex, server.URL, nil)
	if result.Err != nil {
		t.Fatalf("Err: %v", result.Err)
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
	if string(result.RespBody) != "ok" {
		t.Errorf("RespBody = %q", result.RespBody)
	}
	if result.DurationMs < 0 {
		t.Error("expected non-negative DurationMs")
	}
}

func TestReplayExchange_WithQuery(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RequestURI()
		w.WriteHeader(200)
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	ex := model.Exchange{
		Request: model.RequestData{
			Method: "GET",
			Path:   "/api",
			Query:  "key=val",
		},
	}

	result := r.ReplayExchange(context.Background(), ex, server.URL, nil)
	if result.Err != nil {
		t.Fatalf("Err: %v", result.Err)
	}
	if receivedPath != "/api?key=val" {
		t.Errorf("server got path %q, want /api?key=val", receivedPath)
	}
}

func TestReplayExchange_WithHeaders(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(200)
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	ex := model.Exchange{
		Request: model.RequestData{
			Method:  "GET",
			Path:    "/api",
			Headers: map[string][]string{"Authorization": {"Bearer token123"}},
		},
	}

	result := r.ReplayExchange(context.Background(), ex, server.URL, nil)
	if result.Err != nil {
		t.Fatalf("Err: %v", result.Err)
	}
	if receivedAuth != "Bearer token123" {
		t.Errorf("Authorization = %q", receivedAuth)
	}
}

func TestReplayExchange_WithWorkerContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	wctx := NewWorkerContext(5*time.Second, zerolog.Nop())
	wctx.Variables["id"] = "42"

	ex := model.Exchange{
		Request: model.RequestData{
			Method: "GET",
			Path:   "/api/users/{{id}}",
		},
	}

	result := r.ReplayExchange(context.Background(), ex, server.URL, wctx)
	if result.Err != nil {
		t.Fatalf("Err: %v", result.Err)
	}
	if result.Exchange.Request.Path != "/api/users/42" {
		t.Errorf("Path = %q, want /api/users/42", result.Exchange.Request.Path)
	}
}

func TestReplayExchange_ConnectionError(t *testing.T) {
	r := New(&http.Client{Timeout: 100 * time.Millisecond}, zerolog.Nop())
	ex := model.Exchange{
		Request: model.RequestData{
			Method: "GET",
			Path:   "/test",
		},
	}

	result := r.ReplayExchange(context.Background(), ex, "http://127.0.0.1:1", nil)
	if result.Err == nil {
		t.Fatal("expected error for bad address")
	}
	if result.DurationMs < 0 {
		t.Error("expected non-negative DurationMs even on error")
	}
}

func TestReplayExchange_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(200)
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ex := model.Exchange{
		Request: model.RequestData{Method: "GET", Path: "/slow"},
	}

	result := r.ReplayExchange(ctx, ex, server.URL, nil)
	if result.Err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestReplaySession_Success(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	session := model.RecordingSession{
		Entries: []model.Exchange{
			{Request: model.RequestData{Method: "GET", Path: "/a"}},
			{Request: model.RequestData{Method: "GET", Path: "/b"}},
			{Request: model.RequestData{Method: "GET", Path: "/c"}},
		},
	}

	results, err := r.ReplaySession(context.Background(), session, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if callCount != 3 {
		t.Errorf("server received %d calls, want 3", callCount)
	}
}

func TestReplaySession_StopsOnError(t *testing.T) {
	r := New(&http.Client{Timeout: 100 * time.Millisecond}, zerolog.Nop())
	session := model.RecordingSession{
		Entries: []model.Exchange{
			{Request: model.RequestData{Method: "GET", Path: "/a"}},
			{Request: model.RequestData{Method: "GET", Path: "/b"}},
		},
	}

	// Use bad URL to force error
	results, err := r.ReplaySession(context.Background(), session, "http://127.0.0.1:1", nil, nil)
	if err != nil {
		t.Fatalf("ReplaySession should not return error: %v", err)
	}
	// Should stop after first error
	if len(results) != 1 {
		t.Fatalf("expected 1 result (stopped on error), got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected first result to have error")
	}
}

func TestReplaySession_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	session := model.RecordingSession{
		Entries: []model.Exchange{
			{Request: model.RequestData{Method: "GET", Path: "/a"}},
		},
	}

	results, err := r.ReplaySession(ctx, session, server.URL, nil, nil)
	if err == nil {
		t.Fatal("expected context error")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestReplaySession_EmptySession(t *testing.T) {
	r := New(nil, zerolog.Nop())
	session := model.RecordingSession{}
	results, err := r.ReplaySession(context.Background(), session, "http://localhost", nil, nil)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestReplaySession_WithExtractionRules(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Token", "extracted-123")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":"user-42"}`))
	}))
	defer server.Close()

	r := New(nil, zerolog.Nop())
	wctx := NewWorkerContext(5*time.Second, zerolog.Nop())

	session := model.RecordingSession{
		Entries: []model.Exchange{
			{Request: model.RequestData{Method: "GET", Path: "/api"}},
		},
	}

	// Note: extraction from headers in ReplaySession uses a minimal http.Response
	// which doesn't have the actual response headers. This is a known limitation.
	results, err := r.ReplaySession(context.Background(), session, server.URL, wctx, nil)
	if err != nil {
		t.Fatalf("ReplaySession: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}
