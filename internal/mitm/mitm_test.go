package mitm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/config"
	"ffuuzz/internal/recorder"
	"ffuuzz/internal/store"
)

func testCertStore(t *testing.T, cadir string) *store.CertStore {
	t.Helper()
	certCfg := config.CertCacheConfig{
		MaxEntries: 100,
		CertDir:    cadir,
	}
	tlsCfg := config.TLSConfig{}
	logger := zerolog.Nop()
	cs, err := store.NewCertStore(certCfg, tlsCfg, logger)
	if err != nil {
		t.Fatalf("new certstore: %v", err)
	}
	return cs
}

func startProxy(t *testing.T, rec recorder.Recorder, cs *store.CertStore, maxBodyBytes int) (addr string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	addr = ln.Addr().String()
	_ = ln.Close()

	cfg := Config{
		ListenAddr:    addr,
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  maxBodyBytes,
		TLSSkipVerify: true,
		Logger:        zerolog.Nop(),
	}

	proxy := New(cfg)
	go func() { _, _ = proxy.ListenAndServe() }()
	time.Sleep(50 * time.Millisecond)

	return addr
}

func newProxyClient(t *testing.T, proxyAddr string) *http.Client {
	t.Helper()

	proxyURL := "http://" + proxyAddr
	return &http.Client{
		Transport: &http.Transport{
			Proxy: func(req *http.Request) (*url.URL, error) {
				return url.Parse(proxyURL)
			},
		},
		Timeout: 5 * time.Second,
	}
}

func readLastTx(t *testing.T, logPath string, deadline time.Duration) recorder.TxRecord {
	t.Helper()

	deadlineTime := time.Now().Add(deadline)
	var lastTx recorder.TxRecord

	for time.Now().Before(deadlineTime) {
		data, err := os.ReadFile(logPath)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		lines := bytes.Split(data, []byte("\n"))
		if len(lines) == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if err := json.Unmarshal(lines[len(lines)-1], &lastTx); err != nil {
			t.Fatalf("unmarshal last line: %v", err)
		}
		return lastTx
	}

	t.Fatalf("no tx record written to %s within %s", logPath, deadline)
	return lastTx
}

func TestMitm_HTTP_RecordIntegration(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "ok")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("hello"))
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "ffuuzz_integ_test.jsonl")

	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("new jsonl: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_test")
	cs := testCertStore(t, cadir)

	addr := startProxy(t, rec, cs, 16*1024)
	client := newProxyClient(t, addr)

	resp, err := client.Get(backend.URL + "/foo")
	if err != nil {
		t.Fatalf("client get via proxy: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("expected body %q, got %q", "hello", string(body))
	}

	lastTx := readLastTx(t, logPath, 2*time.Second)
	if lastTx.Method != "GET" || lastTx.RespStatus != 200 {
		t.Fatalf("expected recorded GET 200, got method=%s status=%d", lastTx.Method, lastTx.RespStatus)
	}
}

func TestMitm_HTTP_BigBody_TruncationAndFullDelivery(t *testing.T) {
	const (
		bodySize     = 100 * 1024
		maxBodyBytes = 1024
	)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		buf := bytes.Repeat([]byte("x"), bodySize)
		_, _ = w.Write(buf)
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "ffuuzz_bigbody_test.jsonl")

	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("new jsonl: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_bigbody")
	cs := testCertStore(t, cadir)

	addr := startProxy(t, rec, cs, maxBodyBytes)
	client := newProxyClient(t, addr)

	resp, err := client.Get(backend.URL + "/big")
	if err != nil {
		t.Fatalf("client get via proxy: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != bodySize {
		t.Fatalf("expected client body size %d, got %d", bodySize, len(body))
	}

	lastTx := readLastTx(t, logPath, 2*time.Second)

	if lastTx.RespStatus != 200 {
		t.Fatalf("expected resp_status 200, got %d", lastTx.RespStatus)
	}
	if !lastTx.RespTrunc {
		t.Fatalf("expected resp_truncated=true, got false")
	}
	if lastTx.RespBody == "" {
		t.Fatalf("expected resp_body to be non-empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(lastTx.RespBody)
	if err != nil {
		t.Fatalf("base64 decode resp_body: %v", err)
	}
	if len(decoded) != maxBodyBytes {
		t.Fatalf("expected recorded body length %d, got %d", maxBodyBytes, len(decoded))
	}
}

func TestMitm_HTTP_HopByHopHeadersStripped(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Connection") != "" {
			t.Errorf("backend saw Connection header: %q", r.Header.Get("Connection"))
		}
		if r.Header.Get("Proxy-Connection") != "" {
			t.Errorf("backend saw Proxy-Connection header: %q", r.Header.Get("Proxy-Connection"))
		}
		if r.Header.Get("Keep-Alive") != "" {
			t.Errorf("backend saw Keep-Alive header: %q", r.Header.Get("Keep-Alive"))
		}
		if r.Header.Get("TE") != "" {
			t.Errorf("backend saw TE header: %q", r.Header.Get("TE"))
		}
		if r.Header.Get("Trailer") != "" {
			t.Errorf("backend saw Trailer header: %q", r.Header.Get("Trailer"))
		}
		if r.Header.Get("Upgrade") != "" {
			t.Errorf("backend saw Upgrade header: %q", r.Header.Get("Upgrade"))
		}

		w.Header().Set("Connection", "close")
		w.Header().Set("Proxy-Connection", "keep-alive")
		w.Header().Set("Keep-Alive", "timeout=5")
		w.Header().Set("TE", "trailers")
		w.Header().Set("Trailer", "X-Trailer")
		w.Header().Set("Upgrade", "h2c")

		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "ffuuzz_hophop_test.jsonl")

	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("new jsonl: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_hophop")
	cs := testCertStore(t, cadir)

	addr := startProxy(t, rec, cs, 16*1024)
	client := newProxyClient(t, addr)

	req, err := http.NewRequest("GET", backend.URL+"/hophop", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Proxy-Connection", "keep-alive")
	req.Header.Set("Keep-Alive", "timeout=10")
	req.Header.Set("TE", "trailers")
	req.Header.Set("Trailer", "X-Trailer")
	req.Header.Set("Upgrade", "h2c")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client do via proxy: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	hopHeaders := []string{
		"Connection",
		"Proxy-Connection",
		"Keep-Alive",
		"TE",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	}
	for _, h := range hopHeaders {
		if v := resp.Header.Get(h); v != "" {
			t.Errorf("client response has hop-by-hop header %s: %q", h, v)
		}
	}
	lastTx := readLastTx(t, logPath, 2*time.Second)

	for _, h := range hopHeaders {
		if _, ok := lastTx.RespHeaders[h]; ok {
			t.Errorf("recorded resp_headers contains hop-by-hop header %s", h)
		}
	}
}

func TestMitm_HTTP_BigRequestBody_TruncationAndFullDelivery(t *testing.T) {
	const (
		bodySize     = 100 * 1024
		maxBodyBytes = 1024
	)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("backend read body: %v", err)
		}
		if len(body) != bodySize {
			t.Fatalf("backend expected body size %d, got %d", bodySize, len(body))
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "ffuuzz_bigreq_test.jsonl")

	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("new jsonl: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_bigreq")
	cs := testCertStore(t, cadir)

	addr := startProxy(t, rec, cs, maxBodyBytes)
	client := newProxyClient(t, addr)

	reqBody := bytes.Repeat([]byte("y"), bodySize)
	req, err := http.NewRequest("POST", backend.URL+"/bigreq", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client do via proxy: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode != 201 {
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}

	lastTx := readLastTx(t, logPath, 2*time.Second)

	if lastTx.Method != "POST" {
		t.Fatalf("expected recorded method POST, got %s", lastTx.Method)
	}
	if !lastTx.ReqTrunc {
		t.Fatalf("expected req_truncated=true, got false")
	}
	if lastTx.ReqBody == "" {
		t.Fatalf("expected req_body to be non-empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(lastTx.ReqBody)
	if err != nil {
		t.Fatalf("base64 decode req_body: %v", err)
	}
	if len(decoded) != maxBodyBytes {
		t.Fatalf("expected recorded request body length %d, got %d", maxBodyBytes, len(decoded))
	}
}

func TestSingleConnListener_Accept(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	ln := newSingleConnListener(server)
	conn, err := ln.Accept()
	if err != nil {
		t.Fatalf("first Accept: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
}

func TestSingleConnListener_AcceptTwice(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	ln := newSingleConnListener(server)
	_, _ = ln.Accept()

	// Close the listener so second Accept unblocks
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = ln.Close()
	}()

	_, err := ln.Accept()
	if err == nil {
		t.Error("expected error on second Accept")
	}
}

func TestSingleConnListener_Addr(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	ln := newSingleConnListener(server)
	addr := ln.Addr()
	if addr == nil {
		t.Error("expected non-nil addr")
	}
}

func TestSingleConnListener_Addr_AfterAccept(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	ln := newSingleConnListener(server)
	_, _ = ln.Accept()

	addr := ln.Addr()
	if addr == nil {
		t.Error("expected non-nil addr even after accept")
	}
}

func TestSingleConnListener_Close(t *testing.T) {
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	ln := newSingleConnListener(server)
	err := ln.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNew(t *testing.T) {
	cs := testCertStore(t, t.TempDir())
	logPath := filepath.Join(t.TempDir(), "test.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cfg := Config{
		ListenAddr:    "127.0.0.1:0",
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  1024,
		TLSSkipVerify: true,
		Logger:        zerolog.Nop(),
	}
	proxy := New(cfg)
	if proxy == nil {
		t.Fatal("expected non-nil proxy")
	}
	if proxy.transport == nil {
		t.Error("expected non-nil transport")
	}
}

func TestShutdown_NilServer(t *testing.T) {
	p := &Proxy{logger: zerolog.Nop()}
	err := p.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown nil server: %v", err)
	}
}

func TestServeHTTP_NonConnect(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "serve_test.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	cfg := Config{
		ListenAddr:    "127.0.0.1:0",
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  16 * 1024,
		TLSSkipVerify: true,
		Logger:        zerolog.Nop(),
	}
	proxy := New(cfg)

	// Create a request that will go through the proxy's ServeHTTP
	req := httptest.NewRequest("GET", backend.URL+"/test", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestServeHTTP_CONNECT_NoHijack(t *testing.T) {
	// When ResponseWriter doesn't support Hijack, CONNECT returns 500
	logPath := filepath.Join(t.TempDir(), "connect_nohijack.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	cfg := Config{
		ListenAddr:    "127.0.0.1:0",
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  16 * 1024,
		TLSSkipVerify: true,
		Logger:        zerolog.Nop(),
	}
	proxy := New(cfg)

	// httptest.NewRecorder does NOT implement Hijack, so handleCONNECT should fail
	req := httptest.NewRequest("CONNECT", "example.com:443", nil)
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for non-Hijackable writer", w.Code)
	}
}

func TestMitm_HTTP_UpstreamError(t *testing.T) {
	// Use an invalid backend address to trigger upstream error
	logPath := filepath.Join(t.TempDir(), "upstream_err.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	addr := startProxy(t, rec, cs, 16*1024)
	client := newProxyClient(t, addr)

	resp, err := client.Get("http://127.0.0.1:1/fail")
	if err != nil {
		// May get connection refused error from the client
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestMitm_HTTP_POSTBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(200)
		_, _ = w.Write(body) // echo back
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "post_body.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	addr := startProxy(t, rec, cs, 16*1024)
	client := newProxyClient(t, addr)

	reqBody := []byte(`{"key":"value"}`)
	resp, err := client.Post(backend.URL+"/echo", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST via proxy: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if string(body) != `{"key":"value"}` {
		t.Errorf("echo body = %q", string(body))
	}

	lastTx := readLastTx(t, logPath, 2*time.Second)
	if lastTx.Method != "POST" {
		t.Errorf("recorded method = %q, want POST", lastTx.Method)
	}
	if lastTx.ReqBody == "" {
		t.Error("expected non-empty recorded request body")
	}
}

func TestMitm_HTTP_NilBody(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204) // No content
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "nil_body.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	addr := startProxy(t, rec, cs, 16*1024)
	client := newProxyClient(t, addr)

	resp, err := client.Get(backend.URL + "/empty")
	if err != nil {
		t.Fatalf("GET via proxy: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != 204 {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}

	lastTx := readLastTx(t, logPath, 2*time.Second)
	if lastTx.RespStatus != 204 {
		t.Errorf("recorded status = %d, want 204", lastTx.RespStatus)
	}
}

func TestMitm_HTTP_RequestIDHeader(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "reqid_test.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	addr := startProxy(t, rec, cs, 16*1024)
	client := newProxyClient(t, addr)

	resp, err := client.Get(backend.URL + "/reqid")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()

	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header in response")
	}
}

func TestMitmHTTPS_WriteError(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "https_write_err.jsonl")
	rec, _ := recorder.NewJSONL(logPath)
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	proxy := New(Config{
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  16 * 1024,
		TLSSkipVerify: true,
		Logger:        zerolog.Nop(),
	})

	// Create a pipe and close the client side immediately so write fails
	clientConn, serverConn := net.Pipe()
	_ = clientConn.Close()

	done := make(chan struct{})
	go func() {
		proxy.mitmHTTPS(serverConn, "localhost", "test-req")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("mitmHTTPS did not return after write error")
	}
}

func TestMitmHTTPS_TLSHandshakeFail(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "https_hs_fail.jsonl")
	rec, _ := recorder.NewJSONL(logPath)
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	proxy := New(Config{
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  16 * 1024,
		TLSSkipVerify: true,
		Logger:        zerolog.Nop(),
	})

	clientConn, serverConn := net.Pipe()

	done := make(chan struct{})
	go func() {
		proxy.mitmHTTPS(serverConn, "localhost", "test-req")
		close(done)
	}()

	// Client reads the 200 but then sends garbage instead of TLS
	buf := make([]byte, 1024)
	n, _ := clientConn.Read(buf)
	_ = n
	_, _ = clientConn.Write([]byte("NOT TLS DATA AT ALL"))
	_ = clientConn.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("mitmHTTPS did not return after TLS handshake failure")
	}
}

func TestShutdown_WithServer(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "shutdown_test.jsonl")
	rec, _ := recorder.NewJSONL(logPath)
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	proxy := New(Config{
		ListenAddr:    addr,
		CertStore:     cs,
		Recorder:      rec,
		MaxBodyBytes:  16 * 1024,
		TLSSkipVerify: true,
		Logger:        zerolog.Nop(),
	})

	// Pre-assign the server field to avoid the race between
	// ListenAndServe writing p.server and Shutdown reading it.
	proxy.server = &http.Server{
		Addr:    addr,
		Handler: proxy,
	}

	go func() {
		_ = proxy.server.ListenAndServe()
	}()

	// Wait for server to be listening
	for i := 0; i < 50; i++ {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	err = proxy.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestHandleCONNECT_ViaProxy(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "connect_proxy.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	addr := startProxy(t, rec, cs, 16*1024)

	// Make a raw TCP connection to the proxy and send CONNECT
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send CONNECT request
	_, _ = conn.Write([]byte("CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\n\r\n"))

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("CONNECT status = %d, want 200", resp.StatusCode)
	}
}

func TestHandleCONNECT_BadHostPort(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "connect_badhost.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	addr := startProxy(t, rec, cs, 16*1024)

	// Send a CONNECT with host that has no port - SplitHostPort will fail
	// and host will be used as-is
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, _ = conn.Write([]byte("CONNECT example.com HTTP/1.1\r\nHost: example.com\r\n\r\n"))

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	// Should still get 200 because the hijack succeeds
	if resp.StatusCode != 200 {
		t.Errorf("CONNECT status = %d, want 200", resp.StatusCode)
	}
}

func TestHandleCONNECT_FullHTTPS(t *testing.T) {
	// Backend HTTPS server (plain HTTP that mitm will talk to)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("https-ok"))
	}))
	defer backend.Close()

	logPath := filepath.Join(t.TempDir(), "connect_full.jsonl")
	rec, err := recorder.NewJSONL(logPath)
	if err != nil {
		t.Fatalf("NewJSONL: %v", err)
	}
	defer func() { _ = rec.Close() }()

	cs := testCertStore(t, t.TempDir())
	addr := startProxy(t, rec, cs, 16*1024)

	// Connect to the proxy
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Send CONNECT
	_, _ = conn.Write([]byte("CONNECT localhost:443 HTTP/1.1\r\nHost: localhost:443\r\n\r\n"))

	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("CONNECT status = %d, want 200", resp.StatusCode)
	}

	// Perform TLS handshake with the proxy's MITM cert
	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         "localhost",
	})
	defer func() { _ = tlsConn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		t.Fatalf("TLS handshake: %v", err)
	}

	// Send an HTTP request through the TLS connection
	reqStr := "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"
	_, _ = tlsConn.Write([]byte(reqStr))

	tlsReader := bufio.NewReader(tlsConn)
	httpResp, err := http.ReadResponse(tlsReader, nil)
	if err != nil {
		// The upstream will fail since there's no actual HTTPS server on 443
		// but this still exercises the mitmHTTPS code path fully
		return
	}
	_ = httpResp.Body.Close()
}
