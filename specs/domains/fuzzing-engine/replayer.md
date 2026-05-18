# Replayer

## Responsibility

Sends HTTP exchanges to the target server and collects responses. Supports stateful replay via `WorkerContext`, which manages per-worker state: cookie jars, variable extraction from responses, and variable substitution into subsequent requests using `{{var}}` placeholders.

## Key Types

```go
type Replayer struct {
    client  *http.Client
    logger  zerolog.Logger
}

type ExchangeResult struct {
    Exchange   model.Exchange
    StatusCode int
    RespHeaders map[string][]string
    RespBody    []byte
    DurationMs int64
    Err         error
}

type WorkerContext struct {
    CookieJar  *cookiejar.Jar
    Variables  map[string]string
    Client     *http.Client
    Logger     zerolog.Logger
}

type ExtractionRule struct {
    Name   string  // variable name for {{var}} substitution
    Source string  // "body" or "header"
    Header string  // header name (if Source == "header")
    Regex  string  // regex with a capture group
}
```

## Public API

### `New(client *http.Client, logger zerolog.Logger) *Replayer`
Creates a replayer. If `client` is nil, creates a default HTTP client with configurable timeout.

### `ReplayExchange(ctx context.Context, ex model.Exchange, baseURL string, wctx *WorkerContext) (ExchangeResult, error)`
Sends a single exchange to the target. Applies variable substitutions from `wctx.Variables` to the request. Extracts variables from the response according to `wctx` extraction rules. Updates `wctx.CookieJar` from `Set-Cookie` headers.

### `ReplaySession(ctx context.Context, session model.RecordingSession, baseURL string, wctx *WorkerContext, logger zerolog.Logger) ([]ExchangeResult, error)`
Replays all exchanges in a session sequentially. Each exchange sees the state accumulated from previous exchanges (cookies, variables). Used by triage for confirmation and minimization.

## Variable Substitution

The replayer supports `{{var}}` placeholders in request paths, query strings, headers, and bodies:

```
Request: POST /api/orders/{{order_id}}/items
Variables: {"order_id": "12345"}
Replayed: POST /api/orders/12345/items
```

### Extraction

Variables are extracted from responses using regex capture groups defined in `ExtractionRule`:

```go
type ExtractionRule struct {
    Name   string  // "order_id"
    Source string  // "body"
    Regex  string  // `"orderId":\s*"(\d+)"`
}
```

When the response body matches the regex, the capture group value is stored in `wctx.Variables["order_id"]` for use in subsequent requests.

### Per-Worker State

- **CookieJar**: Standard `net/http/cookiejar`. Persists cookies across exchanges within a session. Each worker has its own jar.
- **Variables**: `map[string]string`. Extracted from responses and substituted into subsequent requests.
- **Client**: `*http.Client`. The HTTP client used for replay. Shared across exchanges but per-worker (each worker gets its own client).

## Internal Flow

```
ReplayRequest(exchange):
    │
    ├── Build URL: baseURL + exchange.Request.Path [+ "?" + Query]
    │
    ├── Apply variable substitutions:
    │   └── Replace {{var}} with wctx.Variables[var] in path, query, headers, body
    │
    ├── Create http.Request with method, headers, body
    │
    ├── Apply timeout context (from Config.ReqTimeoutMs)
    │
    ├── client.Do(req) → response
    │
    ├── Read response body (full, no truncation in replay)
    │
    ├── Extract variables from response:
    │   └── For each ExtractionRule: match regex → store in wctx.Variables
    │
    ├── Update cookies:
    │   └── wctx.CookieJar.SetCookies(response cookies)
    │
    └── Return ExchangeResult{Exchange, StatusCode, Headers, Body, DurationMs, Err}
```

## Invariants

- Each worker has its own `WorkerContext`. Variables and cookies are never shared between workers.
- Variable substitution applies to path, query string, headers, and body. If `{{var}}` is not in `Variables`, the placeholder is left unchanged.
- Extraction is opt-in: it runs only when `ExtractionRules` is non-nil.
- Cookie state is maintained per-worker per-session. When replaying a session with `ReplaySession`, cookies from exchange N are available to exchange N+1.
- Replay request timeout is separate from the MITM proxy timeout. It is configured per campaign via `CampaignLimits.ReqTimeoutMs`.
- The replayer does not truncate response bodies. Unlike the MITM proxy (which truncates at `MaxBodyBytes`), replay captures the full body for anomaly detection.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/model` | `Exchange`, `ExtractionRule` |

## Edge Cases

- **Target unreachable**: `ExchangeResult.Err` is set to the connection error. Workers handle this.
- **Variable not found**: `{{missing}}` remains as literal text in the request.
- **Regex doesn't match**: Variable is not stored. No error. Subsequent requests using `{{var}}` won't have the value substituted.
- **Concurrent replay**: Each `ReplayExchange` call is independent. `WorkerContext` is not goroutine-safe — each worker must have its own context.
- **Nil ExtractionRules**: Extraction is skipped. No variables are created.
- **Empty cookie jar**: First request has no cookies. Cookies accumulate from `Set-Cookie` headers in responses.
