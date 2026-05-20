// Package config provides application configuration loading from environment
// variables and CLI flags.
package config

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// TLSConfig configures TLS behavior for the MITM proxy and certificate store.
type TLSConfig struct {
	MinVersion            uint16        `json:"min_version"`
	HandshakeTimeout      time.Duration `json:"handshake_timeout"`
	CipherSuites          []uint16      `json:"cipher_suites,omitempty"`
	DisableSessionTickets bool          `json:"disable_session_tickets"`
}

// CertCacheConfig controls the in-memory LRU and on-disk certificate cache.
type CertCacheConfig struct {
	MaxEntries int    `json:"max_entries"`
	MemoryOnly bool   `json:"memory_only"`
	CertDir    string `json:"cert_dir"`
}

// LLMConfig configures the LLM provider for AI-assisted triage.
type LLMConfig struct {
	Enabled   bool          `json:"enabled"`
	Provider  string        `json:"provider"` // "anthropic" or "openai"
	APIKey    string        `json:"-"`        // never serialized
	BaseURL   string        `json:"base_url"` // optional override
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Timeout   time.Duration `json:"timeout"`
}

// Config holds the complete application configuration.
type Config struct {
	APIAddress      string          `json:"api_address"`
	ProxyAddress    string          `json:"proxy_address"`
	DatabaseURI     string          `json:"database_uri"`
	ArtifactDir     string          `json:"artifact_dir"`
	ReqTimeout      time.Duration   `json:"req_timeout"`
	ShutdownTimeout time.Duration   `json:"shutdown_timeout"`
	Workers         int             `json:"workers"`
	RPS             int             `json:"rps"`
	MaxBodyBytes    int             `json:"max_body_bytes"`
	TLSSkipVerify   bool            `json:"tls_skip_verify"`
	TLS             TLSConfig       `json:"tls"`
	CertCache       CertCacheConfig `json:"cert_cache"`
	LLM             LLMConfig       `json:"llm"`
}

// DefaultConfig returns a Config populated with safe defaults that work for local
// development.
func DefaultConfig() *Config {
	return &Config{
		APIAddress:      ":8081",
		ProxyAddress:    ":8080",
		DatabaseURI:     "postgres://ffuuzz:ffuuzz@localhost:5432/ffuuzz?sslmode=disable",
		ArtifactDir:     "./artifacts",
		ReqTimeout:      3 * time.Second,
		ShutdownTimeout: 30 * time.Second,
		Workers:         8,
		RPS:             50,
		MaxBodyBytes:    64 * 1024,
		TLSSkipVerify:   true,
		TLS: TLSConfig{
			MinVersion:       tls.VersionTLS12,
			HandshakeTimeout: 10 * time.Second,
		},
		CertCache: CertCacheConfig{
			MaxEntries: 1000,
			CertDir:    "certs",
		},
		LLM: LLMConfig{
			Enabled:   false,
			Timeout:   120 * time.Second,
			MaxTokens: 4096,
		},
	}
}

// Load reads configuration from environment variables, then overrides with
// CLI flags from args. Pass os.Args[2:] (after subcommand) for args.
func Load(args []string) (*Config, error) {
	cfg := DefaultConfig()

	// Load .env file if present. Does not override already-set environment
	// variables (real env takes precedence over .env). Supports ${VAR} and
	// $VAR expansion against (a) the OS environment of the running process
	// and (b) variables defined earlier within the .env file itself.
	//
	// Note: godotenv v1.5.1's built-in expansion only consults in-file
	// variables and ignores the OS environment, so we pre-expand OS-env
	// references in the raw file content before handing it to godotenv.
	_ = loadDotEnv(".env")

	// Environment variables (from real env or loaded .env)
	if v := os.Getenv("FFUUZZ_API_ADDRESS"); v != "" {
		cfg.APIAddress = v
	}
	if v := os.Getenv("FFUUZZ_PROXY_ADDRESS"); v != "" {
		cfg.ProxyAddress = v
	}
	if v := os.Getenv("FFUUZZ_DATABASE_URI"); v != "" {
		cfg.DatabaseURI = v
	}
	if v := os.Getenv("FFUUZZ_ARTIFACT_DIR"); v != "" {
		cfg.ArtifactDir = v
	}
	if v := os.Getenv("FFUUZZ_REQ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_REQ_TIMEOUT=%q: %v\n", v, err)
		} else {
			cfg.ReqTimeout = d
		}
	}
	if v := os.Getenv("FFUUZZ_SHUTDOWN_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_SHUTDOWN_TIMEOUT=%q: %v\n", v, err)
		} else {
			cfg.ShutdownTimeout = d
		}
	}
	if v := os.Getenv("FFUUZZ_WORKERS"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_WORKERS=%q: %v\n", v, err)
		} else if n > 0 {
			cfg.Workers = n
		}
	}
	if v := os.Getenv("FFUUZZ_RPS"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_RPS=%q: %v\n", v, err)
		} else if n > 0 {
			cfg.RPS = n
		}
	}
	if v := os.Getenv("FFUUZZ_MAX_BODY_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_MAX_BODY_BYTES=%q: %v\n", v, err)
		} else if n > 0 {
			cfg.MaxBodyBytes = n
		}
	}
	if v := os.Getenv("FFUUZZ_TLS_SKIP_VERIFY"); v != "" {
		if b, err := strconv.ParseBool(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_TLS_SKIP_VERIFY=%q: %v\n", v, err)
		} else {
			cfg.TLSSkipVerify = b
		}
	}
	if v := os.Getenv("FFUUZZ_TLS_MIN_VERSION"); v != "" {
		switch v {
		case "1.2":
			cfg.TLS.MinVersion = tls.VersionTLS12
		case "1.3":
			cfg.TLS.MinVersion = tls.VersionTLS13
		default:
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_TLS_MIN_VERSION=%q (expected 1.2 or 1.3)\n", v)
		}
	}
	if v := os.Getenv("FFUUZZ_TLS_HANDSHAKE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_TLS_HANDSHAKE_TIMEOUT=%q: %v\n", v, err)
		} else {
			cfg.TLS.HandshakeTimeout = d
		}
	}
	if v := os.Getenv("FFUUZZ_TLS_DISABLE_SESSION_TICKETS"); v != "" {
		if b, err := strconv.ParseBool(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_TLS_DISABLE_SESSION_TICKETS=%q: %v\n", v, err)
		} else {
			cfg.TLS.DisableSessionTickets = b
		}
	}
	if v := os.Getenv("FFUUZZ_CERT_CACHE_MAX_ENTRIES"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_CERT_CACHE_MAX_ENTRIES=%q: %v\n", v, err)
		} else if n > 0 {
			cfg.CertCache.MaxEntries = n
		}
	}
	if v := os.Getenv("FFUUZZ_CERT_MEMORY_ONLY"); v != "" {
		if b, err := strconv.ParseBool(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_CERT_MEMORY_ONLY=%q: %v\n", v, err)
		} else {
			cfg.CertCache.MemoryOnly = b
		}
	}
	if v := os.Getenv("FFUUZZ_CERT_CACHE_DIR"); v != "" {
		cfg.CertCache.CertDir = v
	}

	// LLM configuration
	if v := os.Getenv("FFUUZZ_LLM_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_LLM_ENABLED=%q: %v\n", v, err)
		} else {
			cfg.LLM.Enabled = b
		}
	}
	if v := os.Getenv("FFUUZZ_LLM_PROVIDER"); v != "" {
		cfg.LLM.Provider = v
	}
	if v := os.Getenv("FFUUZZ_LLM_API_KEY"); v != "" {
		cfg.LLM.APIKey = v
	}
	if v := os.Getenv("FFUUZZ_LLM_BASE_URL"); v != "" {
		cfg.LLM.BaseURL = v
	}
	if v := os.Getenv("FFUUZZ_LLM_MODEL"); v != "" {
		cfg.LLM.Model = v
	}
	if v := os.Getenv("FFUUZZ_LLM_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_LLM_MAX_TOKENS=%q: %v\n", v, err)
		} else if n > 0 {
			cfg.LLM.MaxTokens = n
		}
	}
	if v := os.Getenv("FFUUZZ_LLM_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_LLM_TIMEOUT=%q: %v\n", v, err)
		} else {
			cfg.LLM.Timeout = d
		}
	}

	// CLI flags override env
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.StringVar(&cfg.APIAddress, "a", cfg.APIAddress, "Control API listen address")
	fs.StringVar(&cfg.ProxyAddress, "p", cfg.ProxyAddress, "MITM proxy listen address")
	fs.StringVar(&cfg.DatabaseURI, "d", cfg.DatabaseURI, "PostgreSQL connection URI")
	fs.StringVar(&cfg.ArtifactDir, "o", cfg.ArtifactDir, "Artifact storage directory")
	fs.StringVar(&cfg.CertCache.CertDir, "cert-dir", cfg.CertCache.CertDir, "Certificate directory")
	fs.IntVar(&cfg.MaxBodyBytes, "max-body", cfg.MaxBodyBytes, "Max body bytes to record")
	fs.IntVar(&cfg.CertCache.MaxEntries, "cert-cache-size", cfg.CertCache.MaxEntries, "Certificate LRU cache max entries")
	fs.BoolVar(&cfg.CertCache.MemoryOnly, "cert-memory-only", cfg.CertCache.MemoryOnly, "Keep certs in memory only (no disk)")
	fs.BoolVar(&cfg.TLS.DisableSessionTickets, "tls-no-tickets", cfg.TLS.DisableSessionTickets, "Disable TLS session tickets")
	fs.BoolVar(&cfg.TLSSkipVerify, "tls-skip-verify", cfg.TLSSkipVerify, "Skip TLS certificate verification for upstream connections")

	// LLM flags
	fs.BoolVar(&cfg.LLM.Enabled, "llm-enabled", cfg.LLM.Enabled, "Enable LLM-assisted triage")
	fs.StringVar(&cfg.LLM.Provider, "llm-provider", cfg.LLM.Provider, "LLM provider: anthropic or openai")
	fs.StringVar(&cfg.LLM.Model, "llm-model", cfg.LLM.Model, "LLM model name")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadDotEnv reads a .env file at path, expands ${VAR}/$VAR references against
// the OS environment of the running process, then parses it with godotenv and
// sets each resulting key in the OS environment if it is not already set.
//
// This wraps godotenv to fix a v1.5.1 limitation: godotenv's built-in variable
// expansion only resolves references against variables defined earlier in the
// same .env file and does NOT consult os.Environ. Pre-expanding the raw file
// content with os.Expand restores the documented behavior of being able to
// reference shell-exported variables (e.g. FFUUZZ_LLM_API_KEY=${OPENAI_API_KEY}).
//
// Returns nil if the file does not exist (the .env file is optional).
func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Pre-expand references that resolve from the OS environment. References
	// to names that are not in the OS environment are preserved verbatim so
	// godotenv can still resolve them against earlier in-file definitions.
	expanded := os.Expand(string(data), func(name string) string {
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		return "${" + name + "}"
	})

	envMap, err := godotenv.Parse(strings.NewReader(expanded))
	if err != nil {
		return err
	}

	for key, value := range envMap {
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return nil
}
