# Recordings

## Overview

Recordings are captured HTTP sessions that serve as seeds for fuzzing campaigns. They are created by the MITM proxy intercepting traffic, stored in PostgreSQL, and organized by target origin (scheme + host + port) and normalized path pattern. The API provides import, export, browsing, and deletion of recordings.

## Key Files

| File | Role |
|------|------|
| `internal/model/model.go` | `RecordingSession`, `Exchange`, `RequestData`, `ResponseData`, `TreeEntry` |
| `internal/recorder/recorder.go` | `Recorder` interface, `TxRecord`, `DBRecorder` |
| `internal/db/recordings.go` | `RecordingStore` PostgreSQL implementation |
| `internal/api/recordings.go` | API handlers for recording CRUD |

## Core Types

```go
type RecordingSession struct {
    SchemaVersion int
    ID            string
    CreatedAt     time.Time
    Target        TargetInfo
    Entries       []Exchange
    EntryCount    int
}

type TargetInfo struct {
    Scheme string
    Host   string
    Port   int
    Path   string
}

type Exchange struct {
    RequestID  string
    StartedAt  time.Time
    DurationMs int64
    Request    RequestData
    Response   ResponseData
}

type TreeEntry struct {
    Scheme string
    Host   string
    Port   int
    Path   string
    Count  int
}
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/recordings/import` | Import a recording session (JSON body — a `RecordingSession` with entries) |
| GET | `/api/v1/recordings/tree` | Get aggregated recording tree grouped by origin+path |
| GET | `/api/v1/recordings/export` | Export recordings filtered by host and prefix (returns `RecordingSession`[] with entries) |
| GET | `/api/v1/recordings` | List recording sessions (no entries, metadata only). Supports `host` and `path_prefix` filters. Paginated (limit default 50, offset 0). |
| GET | `/api/v1/recordings/:id` | Get a single recording with entries. Includes `MaxBodyBytes` truncation. |
| DELETE | `/api/v1/recordings/:id` | Delete a recording. Fails if used by an active campaign. |
| DELETE | `/api/v1/recordings/by-prefix` | Delete recordings matching origin (scheme, host, port) and path prefix. |

## Data Flow

```
MITM Proxy intercepts request/response
    │
    ▼
Recorder.Record(TxRecord)
    │
    ▼
DBRecorder:
  1. Parse URL → scheme, host, port, path
  2. Normalize path (replace params with {_})
  3. resolver.ObservePath(origin, normalizedPath) → may trigger trie collapse
  4. Convert TxRecord → Exchange (Base64 encode body)
  5. FindOrAppend: find existing session by origin+path, append exchange
     OR create new session with one exchange
    │
    ▼
PostgreSQL recording_sessions + exchanges tables
```

## Recording Tree

The tree endpoint groups recordings by origin (scheme, host, port) and path, returning counts:

```
/api/v1/recordings/tree →
[
  {scheme: "https", host: "api.example.com", port: 443, path: "/users/{_}", count: 42},
  {scheme: "https", host: "api.example.com", port: 443, path: "/orders/{_}", count: 15},
  ...
]
```

This mirrors the endpoint normalization: paths with `{_}` representing parameterized segments.

## Invariants

- Recording sessions are grouped by origin (scheme, host, port) and normalized path. Two requests to the same origin and pattern end up in the same session.
- Deleting a recording that is linked to an active (status `RUNNING` or `STARTING`) campaign returns an error.
- Import merges into existing sessions: if a session with the same origin+path exists, exchanges are appended. Otherwise, a new session is created.
- Export returns full sessions (with entries). List returns metadata only (no entries).
- Response body truncation (`MaxBodyBytes`) is applied at recording time (in the MITM proxy via `LimitedBuffer`), not at query time.
- The recording tree query aggregates in SQL using `GROUP BY target_scheme, target_host, target_port, target_path`.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/model` | `RecordingSession`, `Exchange`, `TreeEntry` |
| `internal/db` | `RecordingStore` PostgreSQL implementation, tree query |
| `internal/api` | REST handlers |
| `internal/recorder` | `Recorder` interface, `DBRecorder` |
| `internal/endpoint` | `Resolver` for path normalization and trie |

## Edge Cases

- **Import duplicate**: Appends to existing session — no duplicate session is created.
- **Delete by prefix with no matches**: Returns deleted count 0, no error.
- **Listing returns empty**: Returns HTTP 204 No Content (with pagination headers).
- **Recording linked to active campaign**: Delete returns error with message indicating the campaign ID.
- **Truncated bodies**: Both request and response bodies have `body_truncated` flags. Bodies longer than `MaxBodyBytes` are truncated with the flag set.
- **Empty tree**: Returns empty JSON array `[]`.
