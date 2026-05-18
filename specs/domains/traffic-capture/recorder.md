# Recorder

## Responsibility

Captures HTTP exchanges to persistent storage. Provides the `Recorder` interface consumed by the MITM proxy. Two implementations: `DBRecorder` (PostgreSQL, path-normalised, grouped by origin) and a JSONL file recorder (for the standalone `proxy` CLI command).

## Key Types

```go
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

## Public API

### `Recorder` Interface

```go
Record(tx *TxRecord) error   // Capture a single exchange
Close() error                 // Flush and close
```

Both implementations match this interface. The MITM proxy depends on the interface, not a concrete type.

### `DBRecorder`

```go
func NewDBRecorder(inserter RecordingInserter, resolver *endpoint.Resolver, logger zerolog.Logger) *DBRecorder
```

Created at startup in `cli.runServe()`. Wraps a `RecordingInserter` (implemented by `db.RecordingStore`) and an `endpoint.Resolver` for path normalisation.

```go
type RecordingInserter interface {
    FindOrAppend(ctx context.Context, origin endpoint.Origin,
        normalizedPath string, ex model.Exchange) error
}
```

## Internal Flow (`DBRecorder.Record`)

```
1. Parse URL from TxRecord
   │  ─ Extract scheme, host, port, path, query
   │
2. Normalize path
   │  ─ endpoint.NormalizePath(path)
   │  ─ Replaces UUIDs, numeric IDs with {_}
   │
3. Observe path in Resolver
   │  ─ resolver.ObservePath(origin, normalizedPath)
   │  ─ May trigger trie collapse → async DB merge
   │
4. Convert TxRecord → Exchange
   │  ─ TxRecordToExchange(tx)
   │  ─ Decodes internal representation to model.Exchange
   │  ─ Bodies already Base64-encoded by MITM proxy
   │
5. FindOrAppend in DB
   │  ─ inserter.FindOrAppend(ctx, origin, normalizedPath, exchange)
   │  ─ Finds existing session by origin+path, appends exchange
   │  ─ Creates new session if none exists
   │  ─ Increments metrics.CorpusSize
```

### JSONL File Recorder

Used by the standalone `ffuuzz proxy` command (for development/testing without PostgreSQL). Writes each `TxRecord` as a JSON line to a file. No path normalisation or grouping.

## Conversion Functions

### `TxRecordToExchange(tx *TxRecord) model.Exchange`
Converts from the recording-internal `TxRecord` (flat, metadata-heavy) to the domain `model.Exchange` (typed, body as Base64).

### `ExchangeToTxRecord(ex model.Exchange, baseURL string) *TxRecord`
Reverse conversion. Used by recording export to produce JSONL from the database.

### `EncodeBodyToBase64(body []byte) string`
Utility for manually encoding responses not captured through the MITM proxy pipeline.

## Invariants

- `Record()` is called synchronously per exchange. The MITM proxy blocks on recording completion.
- `FindOrAppend` is atomic from the caller's perspective (handled in the DB layer with `INSERT ... ON CONFLICT` or similar).
- Path normalisation happens before calling `FindOrAppend`. The database stores the normalised path.
- `metrics.CorpusSize` is incremented on every successful `FindOrAppend`, tracking the total number of captured exchanges.
- The `RecordingInserter` interface is defined in `internal/recorder`, consumed by the MITM proxy, and implemented by `internal/db`. This follows the Dependency Inversion Principle.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/endpoint` | `Origin`, `Resolver`, `NormalizePath` |
| `internal/metrics` | `CorpusSize` counter |
| `internal/model` | `Exchange`, `RequestData`, `ResponseData` |

## Edge Cases

- **First recording for an origin+path**: `FindOrAppend` creates a new `RecordingSession` with one exchange.
- **Subsequent recordings**: Exchanges are appended to the existing session.
- **Recording with same request ID**: Behavior depends on `FindOrAppend` implementation (dedup by request ID is handled in the DB layer).
- **Parse error on URL**: `Record()` returns an error. The MITM proxy logs and continues.
