# Triage

## Overview

Handles deduplication, confirmation, minimization, severity scoring, OWASP categorization, and vulnerability grouping of findings discovered during fuzzing. After a worker detects an anomaly, the triage system generates a stable signature to check for duplicates, assigns severity and OWASP category at creation time, re-sends the mutated request N times to confirm the finding is reproducible, re-scores severity with actual reproducibility, and then attempts to minimize the session and body (removing unnecessary exchanges and fields via delta-debugging). Minimization supports JSON, URL-encoded query params, XML, and multipart/form-data bodies.

## Key Files

| File | Role |
|------|------|
| `internal/triage/triage.go` | `Triager` struct with signature, confirm, minimize, severity, categorization, grouping |

## Core Types

```go
type Triager struct{}

// SessionReplayer abstracts ReplaySession for testability.
type SessionReplayer interface {
    ReplaySession(ctx context.Context, session model.RecordingSession,
        baseURL string, wctx *replayer.WorkerContext,
        logger zerolog.Logger) ([]replayer.ExchangeResult, error)
}
```

## Public API

### `Signature(hit AnomalyHit) string`
Computes a deduplication signature: `TYPE|METHOD|normalizedPath|hash(payload)`.
- `TYPE`: finding type (TIMEOUT, SERVER_ERROR, etc.)
- `METHOD`: HTTP method
- `normalizedPath`: path with UUIDs, numeric IDs, and long fuzz segments replaced (`NormalizePath()`)
- `hash(payload)`: first 16 hex chars of SHA-256 of the JSON-normalized request body

Returns a deterministic string usable for database dedup via `ExistsBySignature()`.

### `Confirm(ctx, session, baseURL, detector, anomalyCfg, baseline, rep, runs, timeout, logger) (bool, int, error)`
Replays the entire session `runs` times (default 3). Returns `(confirmed, reproducedCount, error)`. `confirmed` is `true` if the anomaly reproduced in at least `ceil(runs/2)` runs. The `reproduced` count is used to compute `Reproducibility = reproduced / runs` for severity scoring. This is the gate between `UNCONFIRMED` and `CONFIRMED` finding status.

### `ScoreSeverity(findingType, endpoint, method, mutationType, reproducibility, responseStatus) Severity`
Computes severity using a weighted formula: `endpointWeight × typeWeight × mutationTypeWeight × reproducibilityMultiplier`. Returns one of: `CRITICAL`, `HIGH`, `MEDIUM`, `LOW`, `INFO`. Called twice: at finding creation (reproducibility=0, conservative estimate) and after confirmation (actual reproducibility from Confirm runs).

Weight factors:
- **Endpoint weight**: 1.0 for `/auth/` paths, 0.8 for `/admin/` paths, 0.4 default
- **Type weight**: 1.0 (SERVER_ERROR), 0.8 (TIMEOUT), 0.7 (STATUS_CODE_ANOMALY/BEHAVIOR_ANOMALY), 0.5 default
- **Mutation type weight**: 1.0 (primitive), 0.9 (json), 0.7 (header/uri/param), 0.5 default
- **Reproducibility multiplier**: 0.0-0.5 (0-100%), i.e. `reproducibility * 0.5`

Severity thresholds: CRITICAL ≥ 0.8, HIGH ≥ 0.6, MEDIUM ≥ 0.4, LOW ≥ 0.2, INFO < 0.2.

### `CategorizeFinding(findingType, mutationType, responseBody, httpStatus) OWASPCategory`
Maps the finding to an OWASP Top 10 2025 category based on mutation type, response body patterns, and finding type. Categories: `A02_SECURITY_MISCONFIGURATION`, `A04_CRYPTOGRAPHIC_FAILURES`, `A05_INJECTION`, `A06_INSECURE_DESIGN`, `A07_AUTHENTICATION_FAILURES`, `A10_EXCEPTIONAL_CONDITIONS`, or `UNCATEGORIZED`.

### `GroupFindings(findings []Finding) map[string][]Finding`
Groups findings by a composite key: `Type|MutationPrefix|EndpointPattern|HTTPStatusRange`. The endpoint pattern uses only the first path segment (or "root" for "/"). When grouping is applied during campaign stop or periodically every 15s, findings in each group share a common `GroupID`.

### `GetContentType(req RequestData) string`
Extracts the content-type from a request's headers, stripping parameters (e.g., `"application/json; charset=utf-8"` → `"application/json"`). Used to route minimization to the correct strategy.

### `MinimizeQueryParams(ctx, session, exchangeIdx, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) (*RecordingSession, error)`
Applies delta-debugging to URL query parameters in form-encoded bodies (`application/x-www-form-urlencoded`). Splits params into halves, removes one half, and checks if the anomaly still triggers. Returns a session with the minimized body.

### `MinimizeXMLBody(ctx, session, exchangeIdx, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) (*RecordingSession, error)`
Applies delta-debugging to XML elements in XML request bodies. Parses XML into a key-value map, then uses binary-search key removal. Returns a session with the minimized body.

### `MinimizeMultipartBody(ctx, session, exchangeIdx, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) (*RecordingSession, error)`
Applies iterative removal to multipart form data parts. Removes one part at a time, rebuilding the multipart body. Returns a session with the minimized body.

### `MinimizeSession(ctx, session, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) (*RecordingSession, error)`
Tries removing each exchange from the session (starting from the last, never removing the first). After each removal, replays to check if the anomaly still triggers. Returns the minimal session. Sessions with ≤1 exchange are returned as-is.

### `MinimizeJSONBody(ctx, session, exchangeIdx, baseURL, detector, anomalyCfg, baseline, rep, timeout, logger) (*RecordingSession, error)`
Applies binary-search delta-debugging to remove unnecessary JSON keys from the body of the exchange at `exchangeIdx`. Recurses into nested objects up to depth 5. Returns a session with the minimized body, or `nil, nil` if no reduction was possible.

### `NormalizePath(path string) string`
Standalone helper. Replaces UUIDs → `{uuid}`, numeric IDs → `{id}`, and segments ≥ 64 chars → `{fuzz}`.

### `HashPayload(bodyB64 string) string`
Standalone helper. For JSON bodies: decodes, sorts keys, replaces values with type names, re-marshals, and SHA-256 hashes. For non-JSON: hashes the raw bytes.

### `HasJSONBody(ex Exchange) bool`
Standalone helper. Checks content-type header and body structure.

## Flow

```
Worker detects AnomalyHit[]
    │
    ▼
For each hit:
    │
    ├── signature = Signature(hit)
    ├── exists = findings.ExistsBySignature(signature)
    │   └─ YES → skip (dedup)
    │
    ├── severity = ScoreSeverity(type, endpoint, method, mutation, reproducibility=0.0, status)
    ├── category = CategorizeFinding(type, mutationType, responseBody, status)
    ├── create finding → Finding{Status: UNCONFIRMED, Severity, OWASPCategory}
    │
    ├── confirmed, reproduced, err = Confirm(hit.session, ..., runs=confirmRuns)
    │   └─ NO → done (finding stays UNCONFIRMED)
    │
    ├── reproducibility = reproduced / runs
    ├── severity = ScoreSeverity(type, endpoint, method, mutation, reproducibility, status)
    ├── findings.UpdateStatus(id, CONFIRMED)
    │
    ├── minimized = MinimizeSession(hit.session)
    │
    ├── ct = GetContentType(hit.mutatedExchange)
    ├── if ct contains "json":
    │       minimized = MinimizeJSONBody(minimized, exchangeIdx, ...)
    ├── elif ct == "application/x-www-form-urlencoded":
    │       minimized = MinimizeQueryParams(minimized, exchangeIdx, ...)
    ├── elif ct contains "xml":
    │       minimized = MinimizeXMLBody(minimized, exchangeIdx, ...)
    ├── elif ct contains "multipart/form-data":
    │       minimized = MinimizeMultipartBody(minimized, exchangeIdx, ...)
    │
    └── create Artifact with minimized session (update finding severity/reproducibility)
```

## Delta-Debugging Algorithms

All minimization methods use delta-debugging with `stillTriggers()` for verification at each step.

**JSON Body** (`MinimizeJSONBody`):
1. Collect all keys at the current level
2. Split into two halves
3. Try keeping only the right half (removing left keys) — if anomaly still triggers, recurse on right half
4. Try keeping only the left half (removing right keys) — if anomaly still triggers, recurse on left half
5. If neither half alone works, recursively minimize each half independently
6. After minimizing at the current level, recurse into nested objects (up to depth 5)

**Query Parameters** (`MinimizeQueryParams`):
1. Parse URL-encoded body into key-value pairs
2. Apply binary-search key removal via `deltaDebugKeys()`
3. Return minimized body rebuilt from remaining params

**XML Body** (`MinimizeXMLBody`):
1. Parse XML into flat key-value map (shallow, no attributes)
2. Apply binary-search key removal via `deltaDebugKeys()`
3. Re-serialize remaining map back to XML

**Multipart** (`MinimizeMultipartBody`):
1. Parse multipart boundary and extract individual parts
2. Remove one part at a time (not binary-search; simpler removal)
3. Rebuild multipart body with new boundary after each removal

## Severity Scoring Algorithm

`ScoreSeverity` computes an averaged score scaled by reproducibility:

```
score = (endpointWeight + typeWeight + mutationWeight) / 3 × reproMult
```

Weights (all in [0, 1] range):

**Endpoint weight** (default 0.4):
| Pattern | Weight |
|---|---|
| `/auth/` | 1.0 |
| `/admin/` | 0.8 |
| `/api/users` | 0.7 |
| `/api/` | 0.5 |
| `/health` | 0.2 |
| default | 0.4 |

**Type weight** (default 0.4):
| Finding Type | Weight |
|---|---|
| `SERVER_ERROR` | 0.8 |
| `REGEX_MATCH` | 0.6 |
| `TIMEOUT` | 0.5 |
| `LATENCY_REGRESSION` | 0.3 |
| default | 0.4 |

**Mutation weight** (default 0.4): `0.6` for header mutations, `0.5` for uri/param/query mutations, `0.3` for seq mutations, `1.0` for injection-type mutations (sqli, cmdi, xxe, ssrf, template_injection, ldap_injection, xpath_injection).

**ReproMult**: `1.0` when reproducibility ≤ 0 or > 0.8, `0.75` when ≥ 0.5, `0.5` otherwise.

Severity thresholds: `CRITICAL` ≥ 0.8, `HIGH` ≥ 0.6, `MEDIUM` ≥ 0.4, `LOW` ≥ 0.2, `INFO` < 0.2.

## OWASP Categorization

`CategorizeFinding` maps findings to OWASP Top 10 2025 categories using keyword matching on mutation type:

| OWASP Category | Matching Conditions |
|---|---|
| `A05_INJECTION` | Mutation contains sqli, cmdi, xxe, ssrf, template_injection, ldap_injection, or xpath_injection |
| `A04_CRYPTOGRAPHIC_FAILURES` | Mutation contains "jwt" AND HTTP status ≥ 500 |
| `A07_AUTHENTICATION_FAILURES` | Mutation contains "jwt" or "cookie" (non-500 status) |
| `A02_SECURITY_MISCONFIGURATION` | Mutation contains "cors" or "origin" |
| `A10_EXCEPTIONAL_CONDITIONS` | Response body matches stack trace patterns |
| `A06_INSECURE_DESIGN` | Finding type is `SERVER_ERROR` with no specific match |
| `UNCATEGORIZED` | No specific pattern matched |

## Invariants

- Signature computation is deterministic. Same input always produces the same signature.
- `Confirm` requires ≥ ceil(runs/2) reproductions. A single false positive does not prevent confirmation. Returns `(bool, int, error)` — the int is the count of successful reproductions.
- `MinimizeSession` never removes the first exchange (index 0), preserving the initial state-establishing request.
- Body minimization is routed by content-type: `GetContentType()` strips parameters from the content-type header. JSON, URL-encoded, XML, and multipart bodies each have dedicated minimizers. Non-matching content types are skipped.
- Each minimizer (`MinimizeJSONBody`, `MinimizeQueryParams`, `MinimizeXMLBody`, `MinimizeMultipartBody`) returns `nil, nil` when no further reduction is possible (all remaining fields/params/elements are necessary to trigger the anomaly).
- All minimization verifications call `stillTriggers()` which creates a fresh `WorkerContext` for each replay, preventing state leakage between minimization attempts.
- Severity is scored twice: at finding creation (reproducibility=0, conservative) and after confirmation (actual reproducibility). This ensures the final displayed severity reflects real-world reproducibility.
- `GroupFindings` uses a composite key: `Type|MutationPrefix|EndpointPattern|HTTPStatusRange`. The endpoint pattern is always just the first path segment (or "root" for "/").
- Grouping is applied at campaign stop and periodically every 15s via a background goroutine. Only ungrouped confirmed findings (`GroupID == nil`) are processed.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/anomaly` | `AnomalyHit`, `Detector`, `BaselineEntry` |
| `internal/model` | `RecordingSession`, `Exchange`, `AnomalyConfig`, `FindingType`, `Finding`, `Severity`, `OWASPCategory` |
| `internal/replayer` | `ExchangeResult`, `WorkerContext`, `SessionReplayer` |
| `encoding/xml` | XML parsing/serialization for `MinimizeXMLBody` |
| `mime/multipart` | Multipart parsing for `MinimizeMultipartBody` |
| `net/http` | Content-type header parsing for `GetContentType` |

## Edge Cases

- **Session with 1 exchange**: `MinimizeSession` returns it as-is.
- **Non-matching content type**: All body minimizers return `nil, nil` immediately.
- **Context cancelled during minimization**: Returns current state (partial minimization acceptable).
- **All confirmation runs fail**: Finding stays `UNCONFIRMED`. No minimization or artifact creation occurs.
- **Truncated body in minimization**: Returns `nil, nil` (cannot minimize incomplete data).
- **0 reproducibility**: At finding creation, reproMult is 1.0 (no penalty), so initial severity reflects the full weight average without reproducibility reduction.
- **Empty group after filtering**: Periodic grouping skips when < 2 ungrouped findings exist.
- **Single-param query body**: `MinimizeQueryParams` returns `nil, nil` — a single param cannot be reduced.
- **Invalid XML body**: `MinimizeXMLBody` returns `nil, nil` without error (graceful degradation).
- **Invalid multipart body**: `MinimizeMultipartBody` returns `nil, nil` without error (graceful degradation).

## Configuration

| Campaign config field | Type | Default | Effect |
|---|---|---|---|
| `TriageConfig.ConfirmRuns` | int | 0 (min: 3) | Number of replay attempts for confirmation |
| `TriageConfig.EnableMinimization` | bool | false | Enables all minimization (session + content-type-aware body)
