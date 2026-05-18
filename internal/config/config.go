// Package config provides application configuration loading from environment
// variables and CLI flags.
package config

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"
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
	}
}

// Load reads configuration from environment variables, then overrides with
// CLI flags from args. Pass os.Args[2:] (after subcommand) for args.
func Load(args []string) (*Config, error) {
	cfg := DefaultConfig()

	// Environment variables
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
	if v := os.Getenv("FFUUZZ_TLS_SKIP_VERIFY"); v != "" {
		if b, err := strconv.ParseBool(v); err != nil {
			fmt.Fprintf(os.Stderr, "warn: invalid FFUUZZ_TLS_SKIP_VERIFY=%q: %v\n", v, err)
		} else {
			cfg.TLSSkipVerify = b
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

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return cfg, nil
}
