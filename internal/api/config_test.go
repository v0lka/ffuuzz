package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// newTestServerWithEnv creates a test server with a temporary .env file.
// envContent is the initial .env file content; returns the server and cleanup func.
func newTestServerWithEnv(t *testing.T, envContent string) (*Server, func()) {
	t.Helper()

	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if envContent != "" {
		if err := os.WriteFile(envPath, []byte(envContent), 0o600); err != nil {
			t.Fatalf("failed to write temp .env: %v", err)
		}
	}

	srv := NewServer(ServerConfig{
		Addr:       ":0",
		Recordings: &mockRecordingStore{},
		Campaigns:  &mockCampaignStore{},
		Findings:   &mockFindingStore{},
		Artifacts:  &mockArtifactStore{},
		Health:     &mockHealthChecker{},
		Logger:     zerolog.Nop(),
		EnvPath:    envPath,
	})

	cleanup := func() {
		// temp dir cleaned automatically by t.TempDir()
	}

	return srv, cleanup
}

// ---------------------------------------------------------------------------
// GET /api/v1/config tests
// ---------------------------------------------------------------------------

func TestGetConfig_ReturnsDefaultsWhenNoEnvFile(t *testing.T) {
	srv, cleanup := newTestServerWithEnv(t, "")
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp configResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Verify defaults
	if resp.APIAddress != ":8081" {
		t.Errorf("api_address = %q, want :8081", resp.APIAddress)
	}
	if resp.Workers != 8 {
		t.Errorf("workers = %d, want 8", resp.Workers)
	}
	if resp.TLS.MinVersion != "1.2" {
		t.Errorf("tls.min_version = %q, want 1.2", resp.TLS.MinVersion)
	}
}

func TestGetConfig_ReadsEnvFile(t *testing.T) {
	content := `FFUUZZ_API_ADDRESS=:9090
FFUUZZ_WORKERS=16
#FFUUZZ_RPS=100
FFUUZZ_TLS_MIN_VERSION=1.3
`
	srv, cleanup := newTestServerWithEnv(t, content)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp configResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if resp.APIAddress != ":9090" {
		t.Errorf("api_address = %q, want :9090", resp.APIAddress)
	}
	if resp.Workers != 16 {
		t.Errorf("workers = %d, want 16", resp.Workers)
	}
	// Commented lines from .env should fall back to default (NOT parsed by godotenv)
	// Note: godotenv.Parse ignores commented lines
	if resp.RPS != 50 {
		t.Errorf("rps = %d, want 50 (default, commented line ignored)", resp.RPS)
	}
	if resp.TLS.MinVersion != "1.3" {
		t.Errorf("tls.min_version = %q, want 1.3", resp.TLS.MinVersion)
	}
}

func TestGetConfig_MasksAPIKey(t *testing.T) {
	content := `FFUUZZ_LLM_API_KEY=sk-secret-key-12345
`
	srv, cleanup := newTestServerWithEnv(t, content)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp configResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.LLM.APIKey != maskedAPIKey {
		t.Errorf("llm.api_key = %q, want masked sentinel", resp.LLM.APIKey)
	}
}

func TestGetConfig_EmptyAPIKey(t *testing.T) {
	srv, cleanup := newTestServerWithEnv(t, "")
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp configResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.LLM.APIKey != "" {
		t.Errorf("llm.api_key = %q, want empty string", resp.LLM.APIKey)
	}
}

func TestGetConfig_PreservesDurationFormat(t *testing.T) {
	content := `FFUUZZ_REQ_TIMEOUT=5s
`
	srv, cleanup := newTestServerWithEnv(t, content)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp configResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.ReqTimeout != "5s" {
		t.Errorf("req_timeout = %q, want 5s", resp.ReqTimeout)
	}
}

func TestGetConfig_InvalidDurationFallsBackToDefault(t *testing.T) {
	content := `FFUUZZ_REQ_TIMEOUT=not-a-duration
`
	srv, cleanup := newTestServerWithEnv(t, content)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/v1/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp configResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// Should fall back to default (3s)
	if resp.ReqTimeout != "3s" {
		t.Errorf("req_timeout = %q, want 3s (default)", resp.ReqTimeout)
	}
}

// ---------------------------------------------------------------------------
// PUT /api/v1/config tests
// ---------------------------------------------------------------------------

func TestUpdateConfig_ValidatesDuration(t *testing.T) {
	srv, cleanup := newTestServerWithEnv(t, "")
	defer cleanup()

	body := `{"req_timeout": "not-a-duration"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["error"] != "VALIDATION_FAILED" {
		t.Errorf("error = %v, want VALIDATION_FAILED", resp["error"])
	}

	fields, ok := resp["fields"].([]interface{})
	if !ok || len(fields) == 0 {
		t.Fatal("expected fields array in response")
	}
}

func TestUpdateConfig_ValidatesPositiveInt(t *testing.T) {
	srv, cleanup := newTestServerWithEnv(t, "")
	defer cleanup()

	body := `{"workers": -1}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] != "VALIDATION_FAILED" {
		t.Errorf("error = %v, want VALIDATION_FAILED", resp["error"])
	}
}

func TestUpdateConfig_ValidatesEnum(t *testing.T) {
	srv, cleanup := newTestServerWithEnv(t, "")
	defer cleanup()

	body := `{"tls": {"min_version": "1.4"}}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUpdateConfig_WritesToEnvFile(t *testing.T) {
	initial := `# Server
FFUUZZ_API_ADDRESS=:8081

# Performance
FFUUZZ_WORKERS=8
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	body := `{"workers": 16}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	// Read the file back
	data, err := os.ReadFile(srv.envPath)
	if err != nil {
		t.Fatalf("read env: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_WORKERS=16") {
		t.Errorf("expected FFUUZZ_WORKERS=16 in .env, got:\n%s", content)
	}
	// Comment should be preserved
	if !strings.Contains(content, "# Performance") {
		t.Errorf("expected # Performance comment preserved, got:\n%s", content)
	}
	// API address should be unchanged
	if !strings.Contains(content, "FFUUZZ_API_ADDRESS=:8081") {
		t.Errorf("expected FFUUZZ_API_ADDRESS=:8081 unchanged, got:\n%s", content)
	}
}

func TestUpdateConfig_PreservesComments(t *testing.T) {
	initial := `# This is a comment about the server
# FFUUZZ_API_ADDRESS=:8081
FFUUZZ_API_ADDRESS=:8081
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	// Set API address to default — should comment it out
	body := `{"api_address": ":8081"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "# This is a comment") {
		t.Errorf("comments lost:\n%s", content)
	}
}

func TestUpdateConfig_CommentsLineWhenDefault(t *testing.T) {
	initial := `FFUUZZ_WORKERS=16
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	// Set workers back to default (8)
	body := `{"workers": 8}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "#FFUUZZ_WORKERS=8") {
		t.Errorf("expected commented default, got:\n%s", content)
	}
}

func TestUpdateConfig_UncommentsLineWhenNonDefault(t *testing.T) {
	initial := `#FFUUZZ_WORKERS=8
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	body := `{"workers": 16}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_WORKERS=16") {
		t.Errorf("expected active line FFUUZZ_WORKERS=16, got:\n%s", content)
	}
	if strings.Contains(content, "#FFUUZZ_WORKERS") {
		t.Errorf("expected no commented line, got:\n%s", content)
	}
}

func TestUpdateConfig_PreservesVariableExpansion(t *testing.T) {
	initial := `FFUUZZ_LLM_API_KEY=${DEEPSEEK_API_KEY}
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	// Update a different key
	body := `{"workers": 24}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_LLM_API_KEY=${DEEPSEEK_API_KEY}") {
		t.Errorf("variable expansion lost:\n%s", content)
	}
	if !strings.Contains(content, "FFUUZZ_WORKERS=24") {
		t.Errorf("FFUUZZ_WORKERS not updated:\n%s", content)
	}
}

func TestUpdateConfig_SkipsMaskedAPIKey(t *testing.T) {
	initial := `FFUUZZ_LLM_API_KEY=sk-real-key-123
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	// Send masked sentinel — should be skipped
	body := `{"llm": {"api_key": "••••••••"}}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	// Original key should be preserved
	if !strings.Contains(content, "FFUUZZ_LLM_API_KEY=sk-real-key-123") {
		t.Errorf("api key was overwritten with masked sentinel:\n%s", content)
	}
}

func TestUpdateConfig_UpdatesAPIKey(t *testing.T) {
	initial := `FFUUZZ_LLM_API_KEY=sk-old-key
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	body := `{"llm": {"api_key": "sk-new-key"}}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_LLM_API_KEY=sk-new-key") {
		t.Errorf("api key was not updated:\n%s", content)
	}
}

func TestUpdateConfig_AppendsNewKeys(t *testing.T) {
	initial := `FFUUZZ_API_ADDRESS=:9090
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	body := `{"proxy_address": ":8085"}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_PROXY_ADDRESS=:8085") {
		t.Errorf("new key not appended:\n%s", content)
	}
}

func TestUpdateConfig_ReturnsSuccessMessage(t *testing.T) {
	srv, cleanup := newTestServerWithEnv(t, "")
	defer cleanup()

	body := `{"workers": 12}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if msg, ok := resp["message"].(string); !ok || msg == "" {
		t.Errorf("expected success message, got: %v", resp)
	}
}

func TestUpdateConfig_CreatesEnvFileIfMissing(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")

	srv := NewServer(ServerConfig{
		Addr:       ":0",
		Recordings: &mockRecordingStore{},
		Campaigns:  &mockCampaignStore{},
		Findings:   &mockFindingStore{},
		Artifacts:  &mockArtifactStore{},
		Health:     &mockHealthChecker{},
		Logger:     zerolog.Nop(),
		EnvPath:    envPath,
	})

	body := `{"workers": 24}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// File should now exist
	data, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("expected .env file to be created: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "FFUUZZ_WORKERS=24") {
		t.Errorf("expected FFUUZZ_WORKERS=24 in created file, got:\n%s", content)
	}
}

func TestUpdateConfig_NestedTLSFields(t *testing.T) {
	initial := `FFUUZZ_TLS_MIN_VERSION=1.2
FFUUZZ_TLS_HANDSHAKE_TIMEOUT=10s
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	body := `{"tls": {"min_version": "1.3", "handshake_timeout": "15s"}}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_TLS_MIN_VERSION=1.3") {
		t.Errorf("TLS min version not updated:\n%s", content)
	}
	if !strings.Contains(content, "FFUUZZ_TLS_HANDSHAKE_TIMEOUT=15s") {
		t.Errorf("TLS handshake timeout not updated:\n%s", content)
	}
}

func TestUpdateConfig_NestedLLMFields(t *testing.T) {
	initial := `FFUUZZ_LLM_ENABLED=false
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	body := `{"llm": {"enabled": true, "provider": "openai", "max_tokens": 8192}}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_LLM_ENABLED=true") {
		t.Errorf("LLM enabled not updated:\n%s", content)
	}
	if !strings.Contains(content, "FFUUZZ_LLM_PROVIDER=openai") {
		t.Errorf("LLM provider not updated:\n%s", content)
	}
	if !strings.Contains(content, "FFUUZZ_LLM_MAX_TOKENS=8192") {
		t.Errorf("LLM max tokens not updated:\n%s", content)
	}
}

func TestUpdateConfig_NestedCertCacheFields(t *testing.T) {
	initial := `FFUUZZ_CERT_CACHE_MAX_ENTRIES=500
`
	srv, cleanup := newTestServerWithEnv(t, initial)
	defer cleanup()

	body := `{"cert_cache": {"max_entries": 2000, "memory_only": true}}`
	req := httptest.NewRequest("PUT", "/api/v1/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}

	data, _ := os.ReadFile(srv.envPath)
	content := string(data)

	if !strings.Contains(content, "FFUUZZ_CERT_CACHE_MAX_ENTRIES=2000") {
		t.Errorf("cert cache max entries not updated:\n%s", content)
	}
	if !strings.Contains(content, "FFUUZZ_CERT_MEMORY_ONLY=true") {
		t.Errorf("cert memory only not updated:\n%s", content)
	}
}
