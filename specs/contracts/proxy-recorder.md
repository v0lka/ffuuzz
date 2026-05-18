# Proxy → Recorder

## Overview

The MITM proxy records intercepted HTTP exchanges through the `Recorder` interface. The proxy depends on the interface (defined in `internal/recorder`); the `DBRecorder` concrete implementation is injected at startup.

## Interfaces

```go
// internal/recorder/recorder.go

type Recorder interface {
    Record(tx *TxRecord) error
    Close() error
}

type TxRecord struct {
    RequestID   string
    Time        time.Time
    Method      string
    URL         string
    ReqHeaders  map[string][]string
    ReqBody     string              // Base64-encoded
    ReqTrunc    bool
    RespStatus  int
    RespHeaders map[string][]string
    RespBody    string              // Base64-encoded
    RespTrunc   bool
    Timings     map[string]int64
}
```

## Implementations

| Type | Package | Notes |
|------|---------|-------|
| `DBRecorder` | `internal/recorder` | Normalises paths, groups by origin, stores in PostgreSQL |
| `fileRecorder` | `internal/recorder` | Writes JSONL to a file (standalone `proxy` command) |

The `DBRecorder` wraps a `RecordingInserter` interface for the actual DB write:

```go
type RecordingInserter interface {
    FindOrAppend(ctx context.Context, origin endpoint.Origin,
        normalizedPath string, ex model.Exchange) error
}
```

`db.RecordingStore` implements `RecordingInserter` via its `FindOrAppend` method.

## Initialization

In `cli/serve.go`:

```go
rec := recorder.NewDBRecorder(recordingStore, resolver, logger)

proxy := mitm.New(mitm.Config{
    Recorder: rec,  // recorder.Recorder interface
    // ...
})
```

## Data Flow

```
MITM Proxy.ServeHTTP() handles a request:
    │
    ├── Forward request to origin
    ├── Capture response
    │
    ├── Build TxRecord:
    │   ├── RequestID, Time, Method, URL
    │   ├── ReqHeaders (after RemoveHopByHop)
    │   ├── ReqBody (Base64 from LimitedBuffer)
    │   ├── ReqTrunc (true if body > MaxBodyBytes)
    │   ├── RespStatus, RespHeaders
    │   ├── RespBody (Base64 from LimitedBuffer via TeeReadCloser)
    │   ├── RespTrunc
    │   └── Timings (duration_ms, etc.)
    │
    └── cfg.Recorder.Record(tx)
        │
        ├── DBRecorder:
        │   ├── Parse URL → scheme, host, port, path, query
        │   ├── endpoint.NormalizePath(path)
        │   ├── resolver.ObservePath(origin, normalizedPath)
        │   ├── TxRecordToExchange(tx) → model.Exchange
        │   ├── inserter.FindOrAppend(ctx, origin, normalizedPath, exchange)
        │   │   └── PostgreSQL: INSERT/UPDATE recording_sessions + exchanges
        │   └── metrics.CorpusSize.Inc()
        │
        └── fileRecorder (standalone proxy):
            └── Write JSON line to file
```

## Breaking Change Checklist

- [ ] Is the `TxRecord` struct format changing? → update both `DBRecorder` and `fileRecorder`, and `TxRecordToExchange`
- [ ] Is the `Recorder` interface signature changing? → update all implementations and the MITM proxy
- [ ] Is `RecordingInserter` interface changing? → update `db.RecordingStore.FindOrAppend`
- [ ] Are new fields added to `TxRecord`? → ensure they are populated in the MITM proxy before `Record()` is called
- [ ] Is the recording path normalisation logic changing? → verify `endpoint.NormalizePath` and `resolver.ObservePath` compatibility

## Related

- [`domains/traffic-capture/mitm.md`](../domains/traffic-capture/mitm.md) — MITM proxy details
- [`domains/traffic-capture/recorder.md`](../domains/traffic-capture/recorder.md) — recorder details
- [`cli-infrastructure.md`](cli-infrastructure.md) — how these are wired at startup
