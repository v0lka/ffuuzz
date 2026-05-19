# Application Configuration

## Responsibility

Loads application configuration from four sources with a strict priority chain: CLI flags > environment variables (`FFUUZZ_*` via `os.Getenv`) > `.env` file (via `godotenv`, supports `${VAR}` expansion) > `DefaultConfig()` defaults. Invalid values produce warnings to stderr but do not fail the load — the default is used instead.

## Key Files

| File | Role |
|------|------|
| `internal/config/config.go` | `Config` struct, `Load()`, `DefaultConfig()`, all sub-config types |

## Core Types

```go
type Config struct {
    APIAddress      string          // default: ":8081"
    ProxyAddress    string          // default: ":8080"
    DatabaseURI     string          // default: "postgres://ffuuzz:ffuuzz@localhost:5432/ffuuzz?sslmode=disable"
    ArtifactDir     string          // default: "./artifacts"
    ReqTimeout      time.Duration   // default: 3s
    ShutdownTimeout time.Duration   // default: 30s
    Workers         int             // default: 8
    RPS             int             // default: 50
    MaxBodyBytes    int             // default: 64KB (65536)
    TLSSkipVerify   bool            // default: true
    TLS             TLSConfig
    CertCache       CertCacheConfig
    LLM             LLMConfig
}

type TLSConfig struct {
    MinVersion            uint16        // tls.VersionTLS12 or tls.VersionTLS13
    HandshakeTimeout      time.Duration // default: 10s
    CipherSuites          []uint16      // optional, nil = Go defaults
    DisableSessionTickets bool          // default: false
}

type CertCacheConfig struct {
    MaxEntries int    // default: 1000
    MemoryOnly bool   // default: false
    CertDir    string // default: "certs"
}

type LLMConfig struct {
    Enabled   bool          // default: false
    Provider  string        // "anthropic" or "openai"
    APIKey    string        `json:"-"` // never serialized
    BaseURL   string        // optional override for API-compatible proxies
    Model     string        // provider-specific model name
    MaxTokens int           // default: 4096
    Timeout   time.Duration // default: 30s
}
```

## Public API

### `DefaultConfig() *Config`

Returns a `Config` populated with safe local-development defaults. Called once as the base layer before env/flag overrides.

### `Load(args []string) (*Config, error)`

Loads configuration in priority order:

1. `DefaultConfig()` — baseline defaults
2. `.env` file (via `godotenv.Load()`, non-fatal if missing). Supports `${VAR}`/`$VAR` expansion.
3. Environment variables — each `FFUUZZ_*` variable overrides the corresponding field
4. CLI flags — parsed from `args` (passed as `os.Args[2:]` from the `serve` subcommand)

Returns `nil` only on `flag.FlagSet.Parse` failure. All env var parsing errors are non-fatal warnings to stderr.

## Configuration Catalog

### Environment Variables (22 total)

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `FFUUZZ_API_ADDRESS` | string | `:8081` | Control API listen address |
| `FFUUZZ_PROXY_ADDRESS` | string | `:8080` | MITM proxy listen address |
| `FFUUZZ_DATABASE_URI` | string | `postgres://...` | PostgreSQL connection URI |
| `FFUUZZ_ARTIFACT_DIR` | string | `./artifacts` | Artifact storage directory |
| `FFUUZZ_REQ_TIMEOUT` | duration | `3s` | Timeout per replayed request |
| `FFUUZZ_SHUTDOWN_TIMEOUT` | duration | `30s` | Graceful shutdown timeout |
| `FFUUZZ_WORKERS` | int | `8` | Number of fuzz workers |
| `FFUUZZ_RPS` | int | `50` | Max requests per second |
| `FFUUZZ_MAX_BODY_BYTES` | int | `65536` | Max request/response body size |
| `FFUUZZ_TLS_SKIP_VERIFY` | bool | `true` | Skip upstream TLS verification |
| `FFUUZZ_TLS_MIN_VERSION` | `"1.2"` or `"1.3"` | — | Minimum TLS version |
| `FFUUZZ_TLS_HANDSHAKE_TIMEOUT` | duration | `10s` | TLS handshake timeout |
| `FFUUZZ_TLS_DISABLE_SESSION_TICKETS` | bool | `false` | Disable TLS session tickets |
| `FFUUZZ_CERT_CACHE_MAX_ENTRIES` | int | `1000` | LRU cache max certificates |
| `FFUUZZ_CERT_MEMORY_ONLY` | bool | `false` | Keep certs in memory only |
| `FFUUZZ_CERT_CACHE_DIR` | string | `certs` | Certificate storage directory |
| `FFUUZZ_LLM_ENABLED` | bool | `false` | Enable LLM-assisted triage |
| `FFUUZZ_LLM_PROVIDER` | string | — | `"anthropic"` or `"openai"` |
| `FFUUZZ_LLM_API_KEY` | string | — | Provider API key |
| `FFUUZZ_LLM_BASE_URL` | string | — | Optional API base URL override |
| `FFUUZZ_LLM_MODEL` | string | — | Model name (provider-specific) |
| `FFUUZZ_LLM_MAX_TOKENS` | int | `4096` | Max response tokens |
| `FFUUZZ_LLM_TIMEOUT` | duration | `30s` | LLM request timeout |

### CLI Flags (13 total)

| Flag | Shorthand | Default | Description |
|------|-----------|---------|-------------|
| `-a` | `"a"` | `:8081` | Control API listen address |
| `-p` | `"p"` | `:8080` | MITM proxy listen address |
| `-d` | `"d"` | PostgreSQL URI | PostgreSQL connection URI |
| `-o` | `"o"` | `./artifacts` | Artifact storage directory |
| `-cert-dir` | — | `certs` | Certificate directory |
| `-max-body` | — | `65536` | Max body bytes to record |
| `-cert-cache-size` | — | `1000` | Certificate LRU cache max entries |
| `-cert-memory-only` | — | `false` | Keep certs in memory only |
| `-tls-no-tickets` | — | `false` | Disable TLS session tickets |
| `-tls-skip-verify` | — | `true` | Skip upstream TLS verification |
| `-llm-enabled` | — | `false` | Enable LLM-assisted triage |
| `-llm-provider` | — | — | LLM provider (`"anthropic"` or `"openai"`) |
| `-llm-model` | — | — | LLM model name |

## Loading Priority Chain

```
CLI flags  >  env vars  >  .env file  >  DefaultConfig()
(highest)                                          (lowest)
```

- `.env` file is loaded via `godotenv.Load()` which does NOT override already-set environment variables (real env takes precedence over `.env`).
- All `FFUUZZ_*` env var reads use `os.Getenv()` which sees both real env and `.env`-loaded values.
- CLI flags are parsed after env, so they always win.
- The `LLMConfig.APIKey` field has `json:"-"` — it is never serialized to logs or JSON output.

## Invariants

- `Load()` returns `nil` only on `flag.FlagSet.Parse` failure (bad flag syntax). All other errors are warnings to stderr.
- Invalid env var values are logged as warnings and silently use the default. The application never fails to start due to a misconfigured env var.
- `FFUUZZ_WORKERS` and `FFUUZZ_RPS` must be > 0 to override defaults. Zero values are ignored.
- `FFUUZZ_TLS_MIN_VERSION` only accepts `"1.2"` or `"1.3"`. Any other value is ignored with a warning.
- The config package is read-once at startup. No other package reads config after `Load()` returns.

## Dependencies

| Package | Used for |
|---------|----------|
| `flag` | CLI flag parsing via `flag.NewFlagSet("serve", ...)` |
| `os` | `os.Getenv()` for env var reads, `os.Stderr` for warnings |
| `github.com/joho/godotenv` | `.env` file loading with `Load()` |
| `crypto/tls` | `tls.VersionTLS12`, `tls.VersionTLS13` constants |

## Related

- [`llm-providers.md`](llm-providers.md) — LLM provider factory consumes `LLMConfig`
- [`../architecture/security-model.md`](../architecture/security-model.md) — TLS config hardening details
- [`../contracts/cli-infrastructure.md`](../contracts/cli-infrastructure.md) — wiring at startup
