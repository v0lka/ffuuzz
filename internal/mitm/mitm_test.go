package mitm

import (
	"bytes"
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

	"ffuuzz/internal/recorder"
	"ffuuzz/internal/store"
)

func startProxy(t *testing.T, rec recorder.Recorder, cs *store.CertStore, maxBodyBytes int) (addr string) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("get free port: %v", err)
	}
	addr = ln.Addr().String()
	ln.Close()

	cfg := Config{
		ListenAddr:   addr,
		CertStore:    cs,
		Recorder:     rec,
		MaxBodyBytes: maxBodyBytes,
	}

	proxy := New(cfg)
	go func() { _ = proxy.ListenAndServe() }()
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
	defer rec.Close()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_test")
	cs, err := store.NewCertStore(cadir)
	if err != nil {
		t.Fatalf("new certstore: %v", err)
	}

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
	defer rec.Close()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_bigbody")
	cs, err := store.NewCertStore(cadir)
	if err != nil {
		t.Fatalf("new certstore: %v", err)
	}

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
	defer rec.Close()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_hophop")
	cs, err := store.NewCertStore(cadir)
	if err != nil {
		t.Fatalf("new certstore: %v", err)
	}

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

	//хипбайхоп не нужны
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
		defer r.Body.Close()
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
	defer rec.Close()

	cadir := filepath.Join(t.TempDir(), "ffuuzz_certs_bigreq")
	cs, err := store.NewCertStore(cadir)
	if err != nil {
		t.Fatalf("new certstore: %v", err)
	}

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
