package recorder

import (
	"encoding/base64"
	"testing"
	"time"

	"ffuuzz/internal/model"
)

func TestEncodeBodyToBase64(t *testing.T) {
	input := []byte("hello world")
	got := EncodeBodyToBase64(input)
	want := base64.StdEncoding.EncodeToString(input)
	if got != want {
		t.Errorf("EncodeBodyToBase64 = %q, want %q", got, want)
	}
}

func TestEncodeBodyToBase64_Empty(t *testing.T) {
	got := EncodeBodyToBase64(nil)
	if got != "" {
		t.Errorf("EncodeBodyToBase64(nil) = %q, want empty", got)
	}
}

func TestTxRecordToExchange(t *testing.T) {
	now := time.Now()
	tx := TxRecord{
		RequestID:   "req-1",
		Time:        now,
		Method:      "POST",
		URL:         "http://example.com/api/users?limit=10",
		ReqHeaders:  map[string][]string{"Content-Type": {"application/json"}},
		ReqBody:     "dGVzdA==",
		ReqTrunc:    true,
		RespStatus:  201,
		RespHeaders: map[string][]string{"X-Test": {"ok"}},
		RespBody:    "cmVzcA==",
		RespTrunc:   false,
		Timings:     map[string]int64{"total_ms": 150},
	}

	ex := TxRecordToExchange(tx)

	if ex.RequestID != "req-1" {
		t.Errorf("RequestID = %q", ex.RequestID)
	}
	if !ex.StartedAt.Equal(now) {
		t.Errorf("StartedAt = %v, want %v", ex.StartedAt, now)
	}
	if ex.DurationMs != 150 {
		t.Errorf("DurationMs = %d, want 150", ex.DurationMs)
	}
	if ex.Request.Method != "POST" {
		t.Errorf("Method = %q", ex.Request.Method)
	}
	if ex.Request.Path != "/api/users" {
		t.Errorf("Path = %q, want /api/users", ex.Request.Path)
	}
	if ex.Request.Query != "limit=10" {
		t.Errorf("Query = %q, want limit=10", ex.Request.Query)
	}
	if ex.Request.BodyB64 != "dGVzdA==" {
		t.Errorf("BodyB64 = %q", ex.Request.BodyB64)
	}
	if !ex.Request.BodyTruncated {
		t.Error("expected BodyTruncated=true")
	}
	if ex.Response.Status != 201 {
		t.Errorf("Status = %d", ex.Response.Status)
	}
	if ex.Response.BodyB64 != "cmVzcA==" {
		t.Errorf("RespBodyB64 = %q", ex.Response.BodyB64)
	}
}

func TestTxRecordToExchange_NoTimings(t *testing.T) {
	tx := TxRecord{
		URL: "http://example.com/api",
	}
	ex := TxRecordToExchange(tx)
	if ex.DurationMs != 0 {
		t.Errorf("DurationMs = %d, want 0", ex.DurationMs)
	}
}

func TestTxRecordToExchange_InvalidURL(t *testing.T) {
	tx := TxRecord{
		URL: "://invalid",
	}
	ex := TxRecordToExchange(tx)
	if ex.Request.Path != "" {
		t.Errorf("Path = %q, expected empty for invalid URL", ex.Request.Path)
	}
}

func TestExchangeToTxRecord(t *testing.T) {
	now := time.Now()
	ex := model.Exchange{
		RequestID:  "req-2",
		StartedAt:  now,
		DurationMs: 200,
		Request: model.RequestData{
			Method:        "GET",
			Path:          "/api/items",
			Query:         "page=1",
			Headers:       map[string][]string{"Accept": {"application/json"}},
			BodyB64:       "Ym9keQ==",
			BodyTruncated: false,
		},
		Response: model.ResponseData{
			Status:        200,
			Headers:       map[string][]string{"X-Resp": {"val"}},
			BodyB64:       "cmVzcA==",
			BodyTruncated: true,
		},
	}

	tx := ExchangeToTxRecord(ex, "http://example.com")

	if tx.RequestID != "req-2" {
		t.Errorf("RequestID = %q", tx.RequestID)
	}
	if tx.URL != "http://example.com/api/items?page=1" {
		t.Errorf("URL = %q", tx.URL)
	}
	if tx.Method != "GET" {
		t.Errorf("Method = %q", tx.Method)
	}
	if tx.RespStatus != 200 {
		t.Errorf("RespStatus = %d", tx.RespStatus)
	}
	if tx.Timings["total_ms"] != 200 {
		t.Errorf("Timings total_ms = %d", tx.Timings["total_ms"])
	}
}

func TestExchangeToTxRecord_NoQuery(t *testing.T) {
	ex := model.Exchange{
		Request: model.RequestData{
			Method: "GET",
			Path:   "/api/test",
			Query:  "",
		},
	}
	tx := ExchangeToTxRecord(ex, "http://example.com")
	if tx.URL != "http://example.com/api/test" {
		t.Errorf("URL = %q, expected no query string", tx.URL)
	}
}

func TestRoundTrip_TxRecordToExchangeAndBack(t *testing.T) {
	now := time.Now().Truncate(time.Millisecond)
	original := TxRecord{
		RequestID:   "req-rt",
		Time:        now,
		Method:      "PUT",
		URL:         "http://example.com/api/data?key=val",
		ReqHeaders:  map[string][]string{"H": {"v"}},
		ReqBody:     "Ym9keQ==",
		ReqTrunc:    false,
		RespStatus:  200,
		RespHeaders: map[string][]string{"R": {"x"}},
		RespBody:    "cmVz",
		RespTrunc:   false,
		Timings:     map[string]int64{"total_ms": 42},
	}

	ex := TxRecordToExchange(original)
	back := ExchangeToTxRecord(ex, "http://example.com")

	if back.Method != original.Method {
		t.Errorf("Method = %q, want %q", back.Method, original.Method)
	}
	if back.RespStatus != original.RespStatus {
		t.Errorf("RespStatus = %d, want %d", back.RespStatus, original.RespStatus)
	}
	if back.URL != original.URL {
		t.Errorf("URL = %q, want %q", back.URL, original.URL)
	}
}

func TestJSONL_CloseNil(t *testing.T) {
	// Test double-close behavior
	tmp := t.TempDir() + "/test.jsonl"
	rec, err := NewJSONL(tmp)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second close should be safe (f is nil)
	if err := rec.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
