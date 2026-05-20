package config

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.APIAddress != ":8081" {
		t.Errorf("APIAddress = %q, want %q", cfg.APIAddress, ":8081")
	}
	if cfg.ProxyAddress != ":8080" {
		t.Errorf("ProxyAddress = %q, want %q", cfg.ProxyAddress, ":8080")
	}
	if cfg.Workers != 8 {
		t.Errorf("Workers = %d, want 8", cfg.Workers)
	}
	if cfg.RPS != 50 {
		t.Errorf("RPS = %d, want 50", cfg.RPS)
	}
	if cfg.ReqTimeout != 3*time.Second {
		t.Errorf("ReqTimeout = %v, want %v", cfg.ReqTimeout, 3*time.Second)
	}
	if cfg.ShutdownTimeout != 30*time.Second {
		t.Errorf("ShutdownTimeout = %v, want %v", cfg.ShutdownTimeout, 30*time.Second)
	}
	if cfg.MaxBodyBytes != 64*1024 {
		t.Errorf("MaxBodyBytes = %d, want %d", cfg.MaxBodyBytes, 64*1024)
	}
	if cfg.TLS.MinVersion != tls.VersionTLS12 {
		t.Errorf("TLS.MinVersion = %d, want %d", cfg.TLS.MinVersion, tls.VersionTLS12)
	}
	if cfg.TLS.HandshakeTimeout != 10*time.Second {
		t.Errorf("TLS.HandshakeTimeout = %v, want %v", cfg.TLS.HandshakeTimeout, 10*time.Second)
	}
	if cfg.CertCache.MaxEntries != 1000 {
		t.Errorf("CertCache.MaxEntries = %d, want 1000", cfg.CertCache.MaxEntries)
	}
	if cfg.CertCache.CertDir != "certs" {
		t.Errorf("CertCache.CertDir = %q, want %q", cfg.CertCache.CertDir, "certs")
	}
}

func TestLoad_DefaultsOnly(t *testing.T) {
	// Clear all env vars that Load reads
	envVars := []string{
		"FFUUZZ_API_ADDRESS", "FFUUZZ_PROXY_ADDRESS", "FFUUZZ_DATABASE_URI",
		"FFUUZZ_ARTIFACT_DIR", "FFUUZZ_REQ_TIMEOUT", "FFUUZZ_SHUTDOWN_TIMEOUT",
		"FFUUZZ_WORKERS", "FFUUZZ_RPS",
	}
	for _, v := range envVars {
		t.Setenv(v, "")
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIAddress != ":8081" {
		t.Errorf("APIAddress = %q, want default", cfg.APIAddress)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("FFUUZZ_API_ADDRESS", ":9090")
	t.Setenv("FFUUZZ_PROXY_ADDRESS", ":9091")
	t.Setenv("FFUUZZ_DATABASE_URI", "postgres://test:test@localhost/test")
	t.Setenv("FFUUZZ_ARTIFACT_DIR", "/tmp/artifacts")
	t.Setenv("FFUUZZ_REQ_TIMEOUT", "5s")
	t.Setenv("FFUUZZ_SHUTDOWN_TIMEOUT", "60s")
	t.Setenv("FFUUZZ_WORKERS", "16")
	t.Setenv("FFUUZZ_RPS", "100")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIAddress != ":9090" {
		t.Errorf("APIAddress = %q, want %q", cfg.APIAddress, ":9090")
	}
	if cfg.ProxyAddress != ":9091" {
		t.Errorf("ProxyAddress = %q, want %q", cfg.ProxyAddress, ":9091")
	}
	if cfg.DatabaseURI != "postgres://test:test@localhost/test" {
		t.Errorf("DatabaseURI = %q", cfg.DatabaseURI)
	}
	if cfg.ArtifactDir != "/tmp/artifacts" {
		t.Errorf("ArtifactDir = %q", cfg.ArtifactDir)
	}
	if cfg.ReqTimeout != 5*time.Second {
		t.Errorf("ReqTimeout = %v, want 5s", cfg.ReqTimeout)
	}
	if cfg.ShutdownTimeout != 60*time.Second {
		t.Errorf("ShutdownTimeout = %v, want 60s", cfg.ShutdownTimeout)
	}
	if cfg.Workers != 16 {
		t.Errorf("Workers = %d, want 16", cfg.Workers)
	}
	if cfg.RPS != 100 {
		t.Errorf("RPS = %d, want 100", cfg.RPS)
	}
}

func TestLoad_EnvInvalidDuration(t *testing.T) {
	t.Setenv("FFUUZZ_REQ_TIMEOUT", "not-a-duration")
	// Clear others
	for _, v := range []string{"FFUUZZ_API_ADDRESS", "FFUUZZ_PROXY_ADDRESS", "FFUUZZ_DATABASE_URI",
		"FFUUZZ_ARTIFACT_DIR", "FFUUZZ_SHUTDOWN_TIMEOUT", "FFUUZZ_WORKERS", "FFUUZZ_RPS"} {
		_ = os.Unsetenv(v)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Should keep default
	if cfg.ReqTimeout != 3*time.Second {
		t.Errorf("ReqTimeout = %v, want default 3s", cfg.ReqTimeout)
	}
}

func TestLoad_EnvInvalidWorkers(t *testing.T) {
	t.Setenv("FFUUZZ_WORKERS", "abc")
	for _, v := range []string{"FFUUZZ_API_ADDRESS", "FFUUZZ_PROXY_ADDRESS", "FFUUZZ_DATABASE_URI",
		"FFUUZZ_ARTIFACT_DIR", "FFUUZZ_REQ_TIMEOUT", "FFUUZZ_SHUTDOWN_TIMEOUT", "FFUUZZ_RPS"} {
		_ = os.Unsetenv(v)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Workers != 8 {
		t.Errorf("Workers = %d, want default 8", cfg.Workers)
	}
}

func TestLoad_EnvZeroWorkers(t *testing.T) {
	t.Setenv("FFUUZZ_WORKERS", "0")
	for _, v := range []string{"FFUUZZ_API_ADDRESS", "FFUUZZ_PROXY_ADDRESS", "FFUUZZ_DATABASE_URI",
		"FFUUZZ_ARTIFACT_DIR", "FFUUZZ_REQ_TIMEOUT", "FFUUZZ_SHUTDOWN_TIMEOUT", "FFUUZZ_RPS"} {
		_ = os.Unsetenv(v)
	}

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// n <= 0 should keep default
	if cfg.Workers != 8 {
		t.Errorf("Workers = %d, want default 8", cfg.Workers)
	}
}

func TestLoad_CLIFlagsOverride(t *testing.T) {
	for _, v := range []string{"FFUUZZ_API_ADDRESS", "FFUUZZ_PROXY_ADDRESS", "FFUUZZ_DATABASE_URI",
		"FFUUZZ_ARTIFACT_DIR", "FFUUZZ_REQ_TIMEOUT", "FFUUZZ_SHUTDOWN_TIMEOUT", "FFUUZZ_WORKERS", "FFUUZZ_RPS"} {
		_ = os.Unsetenv(v)
	}

	args := []string{"-a", ":7070", "-p", ":7071", "-max-body", "128000"}
	cfg, err := Load(args)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIAddress != ":7070" {
		t.Errorf("APIAddress = %q, want %q", cfg.APIAddress, ":7070")
	}
	if cfg.ProxyAddress != ":7071" {
		t.Errorf("ProxyAddress = %q, want %q", cfg.ProxyAddress, ":7071")
	}
	if cfg.MaxBodyBytes != 128000 {
		t.Errorf("MaxBodyBytes = %d, want 128000", cfg.MaxBodyBytes)
	}
}

func TestLoad_CLIOverridesEnv(t *testing.T) {
	t.Setenv("FFUUZZ_API_ADDRESS", ":9090")

	cfg, err := Load([]string{"-a", ":5050"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIAddress != ":5050" {
		t.Errorf("APIAddress = %q, want CLI override %q", cfg.APIAddress, ":5050")
	}
}

func TestLoad_InvalidFlag(t *testing.T) {
	for _, v := range []string{"FFUUZZ_API_ADDRESS", "FFUUZZ_PROXY_ADDRESS", "FFUUZZ_DATABASE_URI",
		"FFUUZZ_ARTIFACT_DIR", "FFUUZZ_REQ_TIMEOUT", "FFUUZZ_SHUTDOWN_TIMEOUT", "FFUUZZ_WORKERS", "FFUUZZ_RPS"} {
		_ = os.Unsetenv(v)
	}

	_, err := Load([]string{"--unknown-flag"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestLoad_CertCacheFlags(t *testing.T) {
	for _, v := range []string{"FFUUZZ_API_ADDRESS", "FFUUZZ_PROXY_ADDRESS", "FFUUZZ_DATABASE_URI",
		"FFUUZZ_ARTIFACT_DIR", "FFUUZZ_REQ_TIMEOUT", "FFUUZZ_SHUTDOWN_TIMEOUT", "FFUUZZ_WORKERS", "FFUUZZ_RPS"} {
		_ = os.Unsetenv(v)
	}

	cfg, err := Load([]string{"-cert-dir", "/tmp/certs", "-cert-cache-size", "500", "-cert-memory-only", "-tls-no-tickets"})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.CertCache.CertDir != "/tmp/certs" {
		t.Errorf("CertDir = %q, want /tmp/certs", cfg.CertCache.CertDir)
	}
	if cfg.CertCache.MaxEntries != 500 {
		t.Errorf("MaxEntries = %d, want 500", cfg.CertCache.MaxEntries)
	}
	if !cfg.CertCache.MemoryOnly {
		t.Error("expected MemoryOnly=true")
	}
	if !cfg.TLS.DisableSessionTickets {
		t.Error("expected DisableSessionTickets=true")
	}
}

// TestLoadDotEnv_OSEnvExpansion verifies that ${VAR} references in .env are
// expanded against the OS environment of the running process. This is the
// regression test for the godotenv v1.5.1 limitation where its built-in
// variable expansion only consults in-file definitions and silently produces
// empty strings for OS-env references like FFUUZZ_LLM_API_KEY=${OPENAI_KEY}.
func TestLoadDotEnv_OSEnvExpansion(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "INFILE_VAR=hello\n" +
		"FROM_OS=${TEST_DOTENV_OS_VAR}\n" +
		"FROM_INFILE=${INFILE_VAR}_world\n" +
		"UNSET_VAR=${TEST_DOTENV_NOT_SET}\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	t.Setenv("TEST_DOTENV_OS_VAR", "from-shell")
	_ = os.Unsetenv("TEST_DOTENV_NOT_SET")
	_ = os.Unsetenv("INFILE_VAR")
	_ = os.Unsetenv("FROM_OS")
	_ = os.Unsetenv("FROM_INFILE")
	_ = os.Unsetenv("UNSET_VAR")
	t.Cleanup(func() {
		_ = os.Unsetenv("INFILE_VAR")
		_ = os.Unsetenv("FROM_OS")
		_ = os.Unsetenv("FROM_INFILE")
		_ = os.Unsetenv("UNSET_VAR")
	})

	if err := loadDotEnv(envPath); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}

	if got := os.Getenv("FROM_OS"); got != "from-shell" {
		t.Errorf("FROM_OS = %q, want %q (OS-env expansion is broken)", got, "from-shell")
	}
	if got := os.Getenv("FROM_INFILE"); got != "hello_world" {
		t.Errorf("FROM_INFILE = %q, want %q (in-file expansion is broken)", got, "hello_world")
	}
	if got := os.Getenv("UNSET_VAR"); got != "" {
		t.Errorf("UNSET_VAR = %q, want empty", got)
	}
}

func TestLoadDotEnv_DoesNotOverrideExistingEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("TEST_DOTENV_PRESET=from-file\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	t.Setenv("TEST_DOTENV_PRESET", "from-shell")

	if err := loadDotEnv(envPath); err != nil {
		t.Fatalf("loadDotEnv: %v", err)
	}
	if got := os.Getenv("TEST_DOTENV_PRESET"); got != "from-shell" {
		t.Errorf("TEST_DOTENV_PRESET = %q, want %q (real env must take precedence)", got, "from-shell")
	}
}

func TestLoadDotEnv_MissingFileIsNotAnError(t *testing.T) {
	if err := loadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Errorf("loadDotEnv on missing file = %v, want nil", err)
	}
}
