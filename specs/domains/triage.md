# Triage

## Overview

Handles deduplication, confirmation, and minimization of findings discovered during fuzzing. After a worker detects an anomaly, the triage system generates a stable signature to check for duplicates, re-sends the mutated request N times to confirm the finding is reproducible, and then attempts to minimize the session (removing unnecessary exchanges) and JSON body (removing unnecessary fields via delta-debugging).

## Key Files

| File | Role |
|------|------|
| `internal/triage/triage.go` | `Triager` struct with `Signature`, `Confirm`, `MinimizeSession`, `MinimizeJSONBody` |

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

### `Confirm(ctx, session, baseURL, detector, anomalyCfg, baseline, rep, runs, timeout, logger) (bool, error)`
Replays the entire session `runs` times (default 3). Returns `true` if the anomaly reproduced in at least `ceil(runs/2)` runs. This is the gate between `UNCONFIRMED` and `CONFIRMED` finding status.

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
    ├── create finding → Finding{Status: UNCONFIRMED}
    │
    ├── confirmed = Confirm(hit.session, ..., runs=confirmRuns)
    │   └─ NO → done (finding stays UNCONFIRMED)
    │
    ├── findings.UpdateStatus(id, CONFIRMED)
    │
    ├── minimized = MinimizeSession(hit.session)
    │
    ├── if HasJSONBody(hit.mutatedExchange):
    │       minimized = MinimizeJSONBody(minimized, exchangeIdx, ...)
    │
    └── create Artifact with minimized session
```

## Delta-Debugging Algorithm

`MinimizeJSONBody` uses a binary-search approach:

1. Collect all keys at the current level
2. Split into two halves
3. Try keeping only the right half (removing left keys) — if anomaly still triggers, recurse on right half
4. Try keeping only the left half (removing right keys) — if anomaly still triggers, recurse on left half
5. If neither half alone works, recursively minimize each half independently
6. After minimizing at the current level, recurse into nested objects (up to depth 5)

## Invariants

- Signature computation is deterministic. Same input always produces the same signature.
- `Confirm` requires ≥ ceil(runs/2) reproductions. A single false positive does not prevent confirmation.
- `MinimizeSession` never removes the first exchange (index 0), preserving the initial state-establishing request.
- JSON body minimization only applies to exchanges with a non-truncated, valid JSON body that has a `content-type: application/json` (or similar) header.
- `MinimizeJSONBody` returns `nil, nil` when no further reduction is possible (all remaining keys are necessary to trigger the anomaly).
- All minimization verifications call `stillTriggers()` which creates a fresh `WorkerContext` for each replay, preventing state leakage between minimization attempts.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/anomaly` | `AnomalyHit`, `Detector`, `BaselineEntry` |
| `internal/model` | `RecordingSession`, `Exchange`, `AnomalyConfig`, `FindingType` |
| `internal/replayer` | `ExchangeResult`, `WorkerContext`, `SessionReplayer` |

## Edge Cases

- **Session with 1 exchange**: `MinimizeSession` returns it as-is.
- **Non-JSON body**: `MinimizeJSONBody` returns `nil, nil` immediately.
- **Context cancelled during minimization**: Returns current state (partial minimization acceptable).
- **All confirmation runs fail**: Finding stays `UNCONFIRMED`. No minimization or artifact creation occurs.
- **Truncated body in JSON minimization**: Returns `nil, nil` (cannot minimize incomplete data).

## Configuration

| Campaign config field | Type | Default | Effect |
|---|---|---|---|
| `TriageConfig.ConfirmRuns` | int | 0 (min: 3) | Number of replay attempts for confirmation |
| `TriageConfig.EnableMinimization` | bool | false | Enables `MinimizeSession` and `MinimizeJSONBody` |
