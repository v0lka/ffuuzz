package engine

import (
	"encoding/base64"
	"testing"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
	"ffuuzz/internal/replayer"
)

func TestDeepCopyExchange_NilHeaders(t *testing.T) {
	ex := model.Exchange{
		Request:  model.RequestData{Method: "GET", Path: "/test"},
		Response: model.ResponseData{Status: 200},
	}
	copied := deepCopyExchange(ex)
	if copied.Request.Method != "GET" {
		t.Errorf("Method = %q, want GET", copied.Request.Method)
	}
	if copied.Response.Status != 200 {
		t.Errorf("Status = %d, want 200", copied.Response.Status)
	}
	if copied.Request.Headers != nil {
		t.Error("expected nil request headers")
	}
	if copied.Response.Headers != nil {
		t.Error("expected nil response headers")
	}
}

func TestDeepCopyExchange_WithHeaders(t *testing.T) {
	reqHeaders := map[string][]string{
		"Content-Type": {"application/json"},
		"X-Custom":     {"val1", "val2"},
	}
	respHeaders := map[string][]string{
		"Server": {"nginx"},
	}
	ex := model.Exchange{
		Request:  model.RequestData{Method: "POST", Path: "/api", Headers: reqHeaders},
		Response: model.ResponseData{Status: 201, Headers: respHeaders},
	}

	copied := deepCopyExchange(ex)

	// Verify independence: mutate original, check copy is unaffected
	reqHeaders["Content-Type"] = []string{"text/plain"}
	respHeaders["Server"] = []string{"apache"}

	if ct := copied.Request.Headers["Content-Type"][0]; ct != "application/json" {
		t.Errorf("copied request Content-Type = %q, want application/json", ct)
	}
	if srv := copied.Response.Headers["Server"][0]; srv != "nginx" {
		t.Errorf("copied response Server = %q, want nginx", srv)
	}
	if len(copied.Request.Headers["X-Custom"]) != 2 {
		t.Errorf("expected 2 X-Custom values, got %d", len(copied.Request.Headers["X-Custom"]))
	}
}

func TestDeepCopyExchange_OnlyRequestHeaders(t *testing.T) {
	ex := model.Exchange{
		Request:  model.RequestData{Headers: map[string][]string{"A": {"1"}}},
		Response: model.ResponseData{Status: 200},
	}
	copied := deepCopyExchange(ex)
	if copied.Request.Headers == nil {
		t.Fatal("expected non-nil request headers")
	}
	if copied.Response.Headers != nil {
		t.Error("expected nil response headers")
	}
}

func TestDeepCopyExchange_OnlyResponseHeaders(t *testing.T) {
	ex := model.Exchange{
		Request:  model.RequestData{Method: "GET"},
		Response: model.ResponseData{Status: 200, Headers: map[string][]string{"B": {"2"}}},
	}
	copied := deepCopyExchange(ex)
	if copied.Request.Headers != nil {
		t.Error("expected nil request headers")
	}
	if copied.Response.Headers == nil {
		t.Fatal("expected non-nil response headers")
	}
}

func TestExtractMutationPayload_Body(t *testing.T) {
	body := "hello world"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	ex := model.Exchange{
		Request: model.RequestData{BodyB64: encoded},
	}
	result := extractMutationPayload(ex, 200)
	if result != body {
		t.Errorf("payload = %q, want %q", result, body)
	}
}

func TestExtractMutationPayload_BodyTruncated(t *testing.T) {
	body := "this is a long body that will be truncated"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	ex := model.Exchange{
		Request: model.RequestData{BodyB64: encoded},
	}
	result := extractMutationPayload(ex, 10)
	if result != body[:10] {
		t.Errorf("payload = %q, want %q", result, body[:10])
	}
}

func TestExtractMutationPayload_Query(t *testing.T) {
	ex := model.Exchange{
		Request: model.RequestData{Query: "foo=bar&baz=qux"},
	}
	result := extractMutationPayload(ex, 200)
	if result != "foo=bar&baz=qux" {
		t.Errorf("payload = %q, want %q", result, "foo=bar&baz=qux")
	}
}

func TestExtractMutationPayload_QueryTruncated(t *testing.T) {
	ex := model.Exchange{
		Request: model.RequestData{Query: "foo=bar&baz=qux"},
	}
	result := extractMutationPayload(ex, 5)
	if result != "foo=b" {
		t.Errorf("payload = %q, want %q", result, "foo=b")
	}
}

func TestExtractMutationPayload_Empty(t *testing.T) {
	ex := model.Exchange{
		Request: model.RequestData{},
	}
	result := extractMutationPayload(ex, 200)
	if result != "" {
		t.Errorf("payload = %q, want empty", result)
	}
}

func TestExtractMutationPayload_InvalidBase64(t *testing.T) {
	ex := model.Exchange{
		Request: model.RequestData{BodyB64: "!!!not-base64!!!"},
	}
	result := extractMutationPayload(ex, 200)
	// Falls through to query, which is empty
	if result != "" {
		t.Errorf("payload = %q, want empty for invalid base64", result)
	}
}

func TestExtractMutationPayload_BodyPriorityOverQuery(t *testing.T) {
	body := "body content"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	ex := model.Exchange{
		Request: model.RequestData{BodyB64: encoded, Query: "q=1"},
	}
	result := extractMutationPayload(ex, 200)
	if result != body {
		t.Errorf("payload = %q, want body content (should prioritize body over query)", result)
	}
}

func TestBuildResponseData_Basic(t *testing.T) {
	result := replayer.ExchangeResult{
		StatusCode: 200,
		RespHeaders: map[string][]string{
			"Content-Type": {"application/json"},
		},
		RespBody: []byte(`{"ok": true}`),
	}
	resp := buildResponseData(result)
	if resp.Status != 200 {
		t.Errorf("Status = %d, want 200", resp.Status)
	}
	if resp.Headers["Content-Type"][0] != "application/json" {
		t.Errorf("Content-Type = %q", resp.Headers["Content-Type"])
	}
	if resp.BodyB64 == "" {
		t.Error("expected non-empty BodyB64")
	}
	if resp.BodyTruncated {
		t.Error("expected BodyTruncated=false")
	}
}

func TestBuildResponseData_EmptyBody(t *testing.T) {
	result := replayer.ExchangeResult{
		StatusCode: 204,
	}
	resp := buildResponseData(result)
	if resp.Status != 204 {
		t.Errorf("Status = %d, want 204", resp.Status)
	}
	if resp.BodyB64 != "" {
		t.Errorf("expected empty BodyB64, got %q", resp.BodyB64)
	}
}

func TestBuildResponseData_NilHeaders(t *testing.T) {
	result := replayer.ExchangeResult{
		StatusCode: 200,
		RespBody:   []byte("ok"),
	}
	resp := buildResponseData(result)
	if resp.Headers != nil {
		t.Error("expected nil headers when input headers are nil")
	}
}

func TestBuildResponseData_LargeBodyTruncated(t *testing.T) {
	largeBody := make([]byte, 128*1024) // 128KB > 64KB limit
	for i := range largeBody {
		largeBody[i] = 'x'
	}
	result := replayer.ExchangeResult{
		StatusCode: 200,
		RespBody:   largeBody,
	}
	resp := buildResponseData(result)
	if !resp.BodyTruncated {
		t.Error("expected BodyTruncated=true for large body")
	}
	decoded, err := base64.StdEncoding.DecodeString(resp.BodyB64)
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(decoded) != 64*1024 {
		t.Errorf("decoded body length = %d, want %d", len(decoded), 64*1024)
	}
}

func TestNewWorker_Fields(t *testing.T) {
	cfg := WorkerConfig{
		ID:         3,
		CampaignID: "camp-1",
		BaseURL:    "http://localhost:8080",
		Logger:     zerolog.Nop(),
	}
	w := NewWorker(cfg)
	if w.id != 3 {
		t.Errorf("id = %d, want 3", w.id)
	}
	if w.campaignID != "camp-1" {
		t.Errorf("campaignID = %q", w.campaignID)
	}
	if w.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q", w.baseURL)
	}
}
