# Application Configuration

## Responsibility

Loads application configuration from four sources with a strict priority chain: CLI flags > environment variables (`FFUUZZ_*` via `os.Getenv`) > `.env` file (parsed by `godotenv` with a wrapper that pre-expands `${VAR}`/`$VAR` references against the OS environment) > `DefaultConfig()` defaults. Invalid values produce warnings to stderr but do not fail the load — the default is used instead.

## Key Files

| File                        | Role                                                               |
| --------------------------- | ------------------------------------------------------------------ |
| `internal/config/config.go` | `Config` struct, `Load()`, `DefaultConfig()`, all sub-config types |
| `internal/api/config.go`    | `GET/PUT /api/v1/config` handlers, `.env` line-by-line writer, field validators, API request/response types |

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
2. `.env` file (via internal `loadDotEnv()`, non-fatal if missing). Supports `${VAR}`/`$VAR` expansion against (a) the OS environment of the running process and (b) variables defined earlier within the same `.env` file.
3. Environment variables — each `FFUUZZ_*` variable overrides the corresponding field
4. CLI flags — parsed from `args` (passed as `os.Args[2:]` from the `serve` subcommand)

Returns `nil` only on `flag.FlagSet.Parse` failure. All env var parsing errors are non-fatal warnings to stderr.

## Configuration Catalog

### Environment Variables (22 total)

| Variable                             | Type               | Default          | Description                    |
| ------------------------------------ | ------------------ | ---------------- | ------------------------------ |
| `FFUUZZ_API_ADDRESS`                 | string             | `:8081`          | Control API listen address     |
| `FFUUZZ_PROXY_ADDRESS`               | string             | `:8080`          | MITM proxy listen address      |
| `FFUUZZ_DATABASE_URI`                | string             | `postgres://...` | PostgreSQL connection URI      |
| `FFUUZZ_ARTIFACT_DIR`                | string             | `./artifacts`    | Artifact storage directory     |
| `FFUUZZ_REQ_TIMEOUT`                 | duration           | `3s`             | Timeout per replayed request   |
| `FFUUZZ_SHUTDOWN_TIMEOUT`            | duration           | `30s`            | Graceful shutdown timeout      |
| `FFUUZZ_WORKERS`                     | int                | `8`              | Number of fuzz workers         |
| `FFUUZZ_RPS`                         | int                | `50`             | Max requests per second        |
| `FFUUZZ_MAX_BODY_BYTES`              | int                | `65536`          | Max request/response body size |
| `FFUUZZ_TLS_SKIP_VERIFY`             | bool               | `true`           | Skip upstream TLS verification |
| `FFUUZZ_TLS_MIN_VERSION`             | `"1.2"` or `"1.3"` | —                | Minimum TLS version            |
| `FFUUZZ_TLS_HANDSHAKE_TIMEOUT`       | duration           | `10s`            | TLS handshake timeout          |
| `FFUUZZ_TLS_DISABLE_SESSION_TICKETS` | bool               | `false`          | Disable TLS session tickets    |
| `FFUUZZ_CERT_CACHE_MAX_ENTRIES`      | int                | `1000`           | LRU cache max certificates     |
| `FFUUZZ_CERT_MEMORY_ONLY`            | bool               | `false`          | Keep certs in memory only      |
| `FFUUZZ_CERT_CACHE_DIR`              | string             | `certs`          | Certificate storage directory  |
| `FFUUZZ_LLM_ENABLED`                 | bool               | `false`          | Enable LLM-assisted triage     |
| `FFUUZZ_LLM_PROVIDER`                | string             | —                | `"anthropic"` or `"openai"`    |
| `FFUUZZ_LLM_API_KEY`                 | string             | —                | Provider API key               |
| `FFUUZZ_LLM_BASE_URL`                | string             | —                | Optional API base URL override |
| `FFUUZZ_LLM_MODEL`                   | string             | —                | Model name (provider-specific) |
| `FFUUZZ_LLM_MAX_TOKENS`              | int                | `4096`           | Max response tokens            |
| `FFUUZZ_LLM_TIMEOUT`                 | duration           | `30s`            | LLM request timeout            |

### CLI Flags (13 total)

| Flag                | Shorthand | Default        | Description                                |
| ------------------- | --------- | -------------- | ------------------------------------------ |
| `-a`                | `"a"`     | `:8081`        | Control API listen address                 |
| `-p`                | `"p"`     | `:8080`        | MITM proxy listen address                  |
| `-d`                | `"d"`     | PostgreSQL URI | PostgreSQL connection URI                  |
| `-o`                | `"o"`     | `./artifacts`  | Artifact storage directory                 |
| `-cert-dir`         | —         | `certs`        | Certificate directory                      |
| `-max-body`         | —         | `65536`        | Max body bytes to record                   |
| `-cert-cache-size`  | —         | `1000`         | Certificate LRU cache max entries          |
| `-cert-memory-only` | —         | `false`        | Keep certs in memory only                  |
| `-tls-no-tickets`   | —         | `false`        | Disable TLS session tickets                |
| `-tls-skip-verify`  | —         | `true`         | Skip upstream TLS verification             |
| `-llm-enabled`      | —         | `false`        | Enable LLM-assisted triage                 |
| `-llm-provider`     | —         | —              | LLM provider (`"anthropic"` or `"openai"`) |
| `-llm-model`        | —         | —              | LLM model name                             |

## Loading Priority Chain

```
CLI flags  >  env vars  >  .env file  >  DefaultConfig()
(highest)                                          (lowest)
```

- `.env` file is loaded via the internal `loadDotEnv()` wrapper which does NOT override already-set environment variables (real env takes precedence over `.env`).
- `${VAR}` and `$VAR` references in `.env` values are resolved against the OS environment first, then against earlier in-file definitions. The wrapper exists because `godotenv@v1.5.1`'s built-in expansion only consults in-file variables and silently produces empty strings for OS-env references (e.g. `FFUUZZ_LLM_API_KEY=${OPENAI_API_KEY}`).
- All `FFUUZZ_*` env var reads use `os.Getenv()` which sees both real env and `.env`-loaded values.
- CLI flags are parsed after env, so they always win.
- The `LLMConfig.APIKey` field has `json:"-"` — it is never serialized to logs or JSON output.

## Invariants

- `Load()` returns `nil` only on `flag.FlagSet.Parse` failure (bad flag syntax). All other errors are warnings to stderr.
- Invalid env var values are logged as warnings and silently use the default. The application never fails to start due to a misconfigured env var.
- `FFUUZZ_WORKERS` and `FFUUZZ_RPS` must be > 0 to override defaults. Zero values are ignored.
- `FFUUZZ_TLS_MIN_VERSION` only accepts `"1.2"` or `"1.3"`. Any other value is ignored with a warning.
- The config package is read-once at startup. No other package reads config after `Load()` returns.

## Configuration API

The `.env` file is readable and writable at runtime via two REST endpoints in `internal/api/config.go`. These endpoints modify the file on disk but do **not** reload the in-memory configuration or restart any service — changes take effect on next server restart.

### `GET /api/v1/config`

Returns the current configuration as JSON. Values are read from the `.env` file via `godotenv.Parse()`; missing keys fall back to `DefaultConfig()`.

| Serialization difference | Reason |
| --- | --- |
| Durations are strings (`"3s"`) | `.env` stores durations as text, not `time.Duration` |
| `TLS.MinVersion` is `"1.2"` or `"1.3"` | `.env` uses string version labels, not `uint16` constants |
| `LLM.APIKey` is masked (`"••••••••"`) | Keys are never exposed over the API |

### `PUT /api/v1/config`

Accepts a partial JSON body (all fields optional via pointers). Validates each provided field, then updates the `.env` file line-by-line.

| Validation rule | Fields |
| --- | --- |
| Valid Go duration format | `req_timeout`, `shutdown_timeout`, `tls.handshake_timeout`, `llm.timeout` |
| Positive integer (> 0) | `workers`, `rps`, `max_body_bytes`, `cert_cache.max_entries`, `llm.max_tokens` |
| Enum: `"1.2"` or `"1.3"` | `tls.min_version` |
| Enum: `"anthropic"` or `"openai"` | `llm.provider` |

On validation failure, returns `400 VALIDATION_FAILED` with a `fields` array of `{field, message}` objects.

### `.env` File Preservation

The PUT handler updates the `.env` file line-by-line (not rewrite) to preserve:
- **Comments and section headers** — lines not matching `FFUUZZ_*` assignments pass through unchanged
- **Variable expansion** — `${VAR}` and `$VAR` references (e.g. `FFUUZZ_LLM_API_KEY=${OPENAI_API_KEY}`) are preserved unless the user provides a new literal value
- **Default commenting** — values that match the built-in default are written as commented lines (`#FFUUZZ_KEY=value`); non-default values are written as active lines (`FFUUZZ_KEY=value`)

### API Key Masking Round-Trip

The GET response returns `api_key: "••••••••"` when a key is set, `""` when unset. The PUT handler skips updating the key when it receives the masked sentinel — only a new literal value overwrites the existing key. The frontend password field starts empty; an omitted `api_key` field preserves the existing key.

## Dependencies

| Package                    | Used for                                                                                                                                     |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| `flag`                     | CLI flag parsing via `flag.NewFlagSet("serve", ...)`                                                                                         |
| `os`                       | `os.Getenv()` for env var reads, `os.Stderr` for warnings                                                                                    |
| `github.com/joho/godotenv` | `.env` file parsing via `godotenv.Parse()` (raw content is pre-expanded with `os.Expand` before parsing — see `loadDotEnv()` in `config.go`) |
| `crypto/tls`               | `tls.VersionTLS12`, `tls.VersionTLS13` constants                                                                                             |

## Related

- [`llm-providers.md`](llm-providers.md) — LLM provider factory consumes `LLMConfig`
- [`../architecture/security-model.md`](../architecture/security-model.md) — TLS config hardening details
- [`../contracts/cli-infrastructure.md`](../contracts/cli-infrastructure.md) — wiring at startup
