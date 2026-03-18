package httputil

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewRequestID_NonEmpty(t *testing.T) {
	id := NewRequestID()
	if id == "" {
		t.Fatal("expected non-empty request ID")
	}
}

func TestNewRequestID_Unique(t *testing.T) {
	ids := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id := NewRequestID()
		if ids[id] {
			t.Fatalf("duplicate ID: %s", id)
		}
		ids[id] = true
	}
}

func TestNewRequestID_ValidFormat(t *testing.T) {
	id := NewRequestID()
	// NewRequestID format is "YYYYMMDD-UUIDv4", extract and validate the UUID part
	parts := strings.Split(id, "-")
	if len(parts) != 6 { // YYYYMMDD + 5 parts of UUID
		t.Errorf("NewRequestID() has unexpected format: %s", id)
		return
	}
	// Reconstruct UUID from parts[1:] (first part is YYYYMMDD)
	uuidStr := strings.Join(parts[1:], "-")
	if _, err := uuid.Parse(uuidStr); err != nil {
		t.Errorf("NewRequestID() produced invalid UUID: %s, error: %v", id, err)
	}
}

func TestRemoveHopByHop(t *testing.T) {
	h := http.Header{}
	h.Set("Connection", "keep-alive")
	h.Set("Proxy-Connection", "keep-alive")
	h.Set("Keep-Alive", "timeout=5")
	h.Set("TE", "trailers")
	h.Set("Trailer", "X-Trailer")
	h.Set("Transfer-Encoding", "chunked")
	h.Set("Upgrade", "h2c")
	h.Set("X-Custom", "keep-me")

	RemoveHopByHop(h)

	hopHeaders := []string{"Connection", "Proxy-Connection", "Keep-Alive", "TE", "Trailer", "Transfer-Encoding", "Upgrade"}
	for _, hdr := range hopHeaders {
		if v := h.Get(hdr); v != "" {
			t.Errorf("expected %s to be removed, got %q", hdr, v)
		}
	}
	if h.Get("X-Custom") != "keep-me" {
		t.Error("X-Custom should not be removed")
	}
}

func TestCopyHeaders(t *testing.T) {
	src := http.Header{}
	src.Set("Content-Type", "application/json")
	src.Add("X-Multi", "a")
	src.Add("X-Multi", "b")

	dst := http.Header{}
	CopyHeaders(dst, src)

	if dst.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", dst.Get("Content-Type"))
	}
	vals := dst.Values("X-Multi")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values for X-Multi, got %d", len(vals))
	}
}

func TestNewLimitedBuffer(t *testing.T) {
	lb := NewLimitedBuffer(10)
	if lb == nil {
		t.Fatal("expected non-nil buffer")
	}
	if lb.Truncated() {
		t.Error("new buffer should not be truncated")
	}
	if len(lb.Bytes()) != 0 {
		t.Error("new buffer should be empty")
	}
}

func TestLimitedBuffer_WriteWithinLimit(t *testing.T) {
	lb := NewLimitedBuffer(10)
	n, err := lb.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 5 {
		t.Errorf("n = %d, want 5", n)
	}
	if string(lb.Bytes()) != "hello" {
		t.Errorf("Bytes = %q, want %q", lb.Bytes(), "hello")
	}
	if lb.Truncated() {
		t.Error("should not be truncated")
	}
}

func TestLimitedBuffer_WriteExceedsLimit(t *testing.T) {
	lb := NewLimitedBuffer(5)
	n, err := lb.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 11 {
		t.Errorf("n = %d, want 11 (always reports full write)", n)
	}
	if len(lb.Bytes()) != 5 {
		t.Errorf("Bytes length = %d, want 5", len(lb.Bytes()))
	}
	if !lb.Truncated() {
		t.Error("should be truncated")
	}
}

func TestLimitedBuffer_MultipleWrites(t *testing.T) {
	lb := NewLimitedBuffer(10)
	_, _ = lb.Write([]byte("abc"))
	_, _ = lb.Write([]byte("defgh"))
	_, _ = lb.Write([]byte("ijk"))
	if len(lb.Bytes()) != 10 {
		t.Errorf("Bytes length = %d, want 10", len(lb.Bytes()))
	}
	if !lb.Truncated() {
		t.Error("should be truncated")
	}
}

func TestLimitedBuffer_ZeroLimit(t *testing.T) {
	// When limit <= 0, LimitedBuffer has no limit (unlimited)
	lb := NewLimitedBuffer(0)
	n, err := lb.Write([]byte("anything"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 8 {
		t.Errorf("n = %d, want 8", n)
	}
	if len(lb.Bytes()) != 8 {
		t.Errorf("Bytes length = %d, want 8 (unlimited)", len(lb.Bytes()))
	}
	if lb.Truncated() {
		t.Error("should not be truncated with no limit")
	}
}

func TestNewTeeReadCloser(t *testing.T) {
	original := io.NopCloser(strings.NewReader("test data"))
	lb := NewLimitedBuffer(100)
	trc := NewTeeReadCloser(original, lb)

	data, err := io.ReadAll(trc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("data = %q, want %q", data, "test data")
	}
	if string(lb.Bytes()) != "test data" {
		t.Errorf("buffer = %q, want %q", lb.Bytes(), "test data")
	}
}

func TestNewTeeReadCloser_Close(t *testing.T) {
	original := io.NopCloser(strings.NewReader("data"))
	lb := NewLimitedBuffer(100)
	trc := NewTeeReadCloser(original, lb)
	if err := trc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewHTTPServer(t *testing.T) {
	params := ServerParams{
		Addr:              ":0",
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: 5,
		ReadTimeout:       10,
		WriteTimeout:      10,
		IdleTimeout:       30,
	}
	srv := NewHTTPServer(params)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.Addr != ":0" {
		t.Errorf("Addr = %q, want %q", srv.Addr, ":0")
	}
}
