# Traffic Capture

## Overview

The traffic capture domain intercepts real HTTP/HTTPS traffic, records it to PostgreSQL, and normalizes endpoint path patterns. It is the "record" phase of FFUUZZ's record-then-fuzz workflow. Components: MITM proxy (intercepts), Recorder (persists with path normalization), Endpoint Resolver (detects parameterized path patterns and triggers DB merges).

## Key Files

| File | Role |
|------|------|
| `internal/mitm/mitm.go` | MITM proxy: CONNECT tunneling, TLS interception, recording |
| `internal/recorder/recorder.go` | `Recorder` interface, `TxRecord`, `DBRecorder` |
| `internal/endpoint/resolver.go` | `Resolver`: path trie, collapse detection, `Merger` interface |
| `internal/endpoint/normalize.go` | `NormalizePath()` heuristic path normalisation |
| `internal/endpoint/trie.go` | `trieNode` segment trie with cardinality tracking |

## Core Types

```go
// Recorder captures HTTP exchanges to storage.
type Recorder interface {
    Record(tx *TxRecord) error
    Close() error
}

// Origin identifies a target by scheme, host, and port.
type Origin struct {
    Scheme string
    Host   string
    Port   int
}
```

## Flow

```
┌──────────────────┐     ┌────────────────┐     ┌───────────────────┐
│   MITM Proxy     │────►│   Recorder     │────►│  Endpoint Resolver │
│  (:8080)         │     │  (DBRecorder)  │     │  (Trie + Merger)  │
│                  │     │                │     │                   │
│ HTTP:            │     │ 1. Parse URL   │     │ 1. ObservePath()  │
│  read → forward  │     │ 2. Normalize   │     │ 2. Update trie    │
│  → record        │     │    path        │     │ 3. Check cardinality│
│                  │     │ 3. Convert     │     │ 4. Collapse if    │
│ HTTPS:           │     │    TxRecord    │     │    threshold met  │
│  CONNECT → TLS   │     │    → Exchange  │     │ 5. DB merge via   │
│  → tunnel →      │     │ 4. FindOrAppend│     │    Merger interface│
│  record          │     │    in DB       │     │                   │
└──────────────────┘     └────────────────┘     └───────────────────┘
```

The MITM proxy calls `Recorder.Record()` for every captured exchange. The `DBRecorder` normalises the path via `NormalizePath()` and observes it in the `Resolver`, which may trigger a statistical collapse if enough recordings share a parameterized path segment.

## Components

- [`mitm.md`](mitm.md) — MITM proxy: HTTP handling, CONNECT hijacking, TLS interception
- [`recorder.md`](recorder.md) — Recorder interface, DBRecorder, JSONL file recorder
- [`endpoint.md`](endpoint.md) — Path normalisation, segment trie, collapse detection

## Invariants

- Recording and endpoint normalization happen synchronously within the `Record()` call. The proxy waits for the recording to complete before handling the next request.
- The `Resolver` operates per-origin. A trie tracks paths for `https://api.example.com:443` independently from `https://api.example.com:8443`.
- `NormalizePath()` is a heuristic-only operation. It never makes database queries. It uses regex patterns to replace UUIDs, numeric IDs, and timestamp segments with `{_}`.
- Endpoint collapse triggers an asynchronous DB merge (via `Merger.MergeRecordings`). The merge is fire-and-forget: the recording has already succeeded regardless of merge outcome.
- The resolver is rebuilt from DB on startup (`RebuildFromDB`). If rebuild fails, the proxy starts with an empty trie and rebuilds it as traffic arrives.

## Configuration

| Config field | Type | Default | Effect |
|---|---|---|---|
| `Config.ProxyAddress` | string | `:8080` | MITM proxy listen address |
| `Config.MaxBodyBytes` | int | 65536 | Maximum request/response body size to record |
| `Config.TLSSkipVerify` | bool | true | Skip TLS verification for upstream connections |
