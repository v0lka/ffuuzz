# Report Generation

## Responsibility

Builds aggregate summary statistics from a set of recorded HTTP transactions (`TxRecord` from `internal/recorder`). Produces breakdowns by HTTP method, response status code, and target host.

## Key Files

| File | Role |
|------|------|
| `internal/report/report.go` | `Summary`, `BuildSummary` |

## Core Types

```go
// Summary aggregates statistics from a set of recorded transactions.
type Summary struct {
    Total    int            `json:"total"`
    ByMethod map[string]int `json:"by_method"` // method → count
    ByStatus map[int]int    `json:"by_status"` // status code → count
    ByHost   map[string]int `json:"by_host"`   // hostname → count
}
```

## Public API

### `BuildSummary(records []recorder.TxRecord) Summary`

Aggregates statistics from a slice of recorded transactions:

1. Increments `Total` for every record (including records with invalid URLs)
2. `ByMethod[tx.Method]++` for each record
3. `ByStatus[tx.RespStatus]++` for each record
4. Parses `tx.URL` to extract the host. If parsing succeeds and `u.Host` is not empty, `ByHost[u.Host]++`. Invalid URLs are silently skipped in host counting.

Returns a `Summary` struct with all maps initialized (never nil), even when `records` is empty.

## Internal Flow

```
BuildSummary(records):
    │
    ├── Initialize Summary with empty maps (ByMethod, ByStatus, ByHost)
    │
    ├── For each tx in records:
    │   ├── s.Total++
    │   ├── s.ByMethod[tx.Method]++
    │   ├── s.ByStatus[tx.RespStatus]++
    │   └── Parse tx.URL:
    │       ├── OK and u.Host != "" → s.ByHost[u.Host]++
    │       └── Error or empty host → skip (no error, no warning)
    │
    └── Return Summary
```

## Status Code Grouping

Status codes are recorded individually (not grouped into 2xx/3xx/4xx/5xx buckets). Each distinct status code gets its own entry in `ByStatus`. For example, a set of responses with statuses 200, 201, 400, 500 would produce:

```json
{
    "by_status": {
        "200": 10,
        "201": 3,
        "400": 2,
        "500": 1
    }
}
```

## Invariants

- All maps (`ByMethod`, `ByStatus`, `ByHost`) are always initialized via `make()`. They are never nil, even for empty input.
- `Total` counts every record in the input, including those with invalid URLs that are skipped in `ByHost`.
- Invalid URLs are silently skipped. No error is returned and no warning is logged.
- `ByHost` uses the host portion of the URL only (no scheme, no port, no path). If the URL has no host component, it is skipped.
- `BuildSummary` is a pure function. Output depends only on the input slice.

## Dependencies

| Package | Used for |
|---------|----------|
| `net/url` | `url.Parse()` for hostname extraction |
| `internal/recorder` | `TxRecord` type for input |

## Edge Cases

- **Empty input**: Returns `Summary{Total: 0}` with empty (but not nil) maps.
- **Invalid URL**: Record is counted in `Total`, `ByMethod`, `ByStatus` but NOT in `ByHost`.
- **URL with no host** (e.g., `"/relative/path"`): Same as invalid URL — skipped in `ByHost`.
- **URL with port**: `ByHost` includes only the hostname portion (e.g., `"api.example.com"` for `"https://api.example.com:443/path"`).
- **Large input**: O(n) time and O(unique methods + unique statuses + unique hosts) space.

## Related

- [`../domains/traffic-capture/recorder.md`](../domains/traffic-capture/recorder.md) — `TxRecord` type definition
- [`../architecture/layers.md`](../architecture/layers.md) — utility layer classification
