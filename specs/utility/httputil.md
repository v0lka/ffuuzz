# HTTP Utilities

## Responsibility

Provides shared HTTP helpers used by the MITM proxy and API server: hop-by-hop header stripping, size-limited body buffering with truncation tracking, tee readers for mirrored reads, header copying, request ID generation, and HTTP server construction.

## Key Files

| File | Role |
|------|------|
| `internal/httputil/http.go` | `RemoveHopByHop`, `CopyHeaders`, `LimitedBuffer`, `TeeReadCloser` |
| `internal/httputil/server.go` | `ServerParams`, `NewHTTPServer` |
| `internal/httputil/reqid.go` | `NewRequestID` |

## Core Types

```go
type LimitedBuffer struct { ... }
// Size-limited byte buffer that tracks whether data was truncated.
// Zero or negative limit disables truncation.

type TeeReadCloser struct { ... }
// Mirrors reads to a secondary writer while preserving close semantics.
// Returns the original reader when either input is nil.

type ServerParams struct {
    Addr              string
    Handler           http.Handler
    ReadTimeout       time.Duration
    ReadHeaderTimeout time.Duration
    WriteTimeout      time.Duration
    IdleTimeout       time.Duration
}
```

## Public API

### `RemoveHopByHop(h http.Header)`

Strips hop-by-hop headers from the given header map. Removes: `Connection`, `Proxy-Connection`, `Keep-Alive`, `TE`, `Trailer`, `Transfer-Encoding`, `Upgrade`. Called by the MITM proxy before recording (but after forwarding to origin).

### `CopyHeaders(dst, src http.Header)`

Deep-copies all header values from `src` to `dst` using `dst.Add()`. Preserves multi-value headers.

### `NewLimitedBuffer(limit int) *LimitedBuffer`

Creates a buffer that accepts up to `limit` bytes. When `limit <= 0`, the buffer is unlimited.

Methods:
- `Write(p []byte) (int, error)` — writes up to the limit. Always reports `len(p)` written even when truncated (avoids confusing callers expecting a full write).
- `Bytes() []byte` — returns the buffered bytes.
- `Truncated() bool` — returns `true` if the limit was exceeded.

### `NewTeeReadCloser(r io.ReadCloser, w io.Writer) io.ReadCloser`

Returns a reader that mirrors all reads to `w`. When `r` or `w` is `nil`, returns `r` directly (no-op tee). The `Close()` method delegates to `r.Close()`.

### `NewHTTPServer(p ServerParams) *http.Server`

Constructs a `*http.Server` with timeout configuration from `ServerParams`.

### `NewRequestID() string`

Generates a request ID in the format `YYYYMMDD-UUIDv4` (e.g., `20260520-550e8400-e29b-41d4-a716-446655440000`).

## Internal Flow

### LimitedBuffer Write Semantics

```
Write(p):
    if limit <= 0:
        write all → buf (unlimited)
        return len(p), nil
    remaining = limit - buf.Len()
    if remaining > 0:
        if len(p) <= remaining:
            write all → buf
        else:
            write p[:remaining] → buf
            truncated = true
    else:
        truncated = true (already at limit)
    return len(p), nil  // always reports full write
```

### TeeReadCloser Read Semantics

```
Read(p):
    n, err = r.Read(p)
    if n > 0:
        w.Write(p[:n])  // mirror bytes
    return n, err
Close():
    return r.Close()
```

### Hop-by-Hop Header List

The complete list of headers removed by `RemoveHopByHop`:

```
Connection
Proxy-Connection
Keep-Alive
TE
Trailer
Transfer-Encoding
Upgrade
```

These are stripped from the recorded `TxRecord` but the original headers are sent to the upstream server.

## Invariants

- `LimitedBuffer.Write` always returns `len(p), nil`. It never fails a write — truncation is silent to the caller. Callers must check `Truncated()` explicitly.
- `LimitedBuffer` does not align to any boundary. Truncation may occur mid-byte.
- `NewTeeReadCloser(r, nil)` returns `r` directly. No wrapper is created. This allows conditional tee usage without extra allocations.
- `NewTeeReadCloser(nil, w)` returns `nil`. The original reader is the zero value `r`.
- `RemoveHopByHop` mutates the header map in place. Callers should copy headers before calling if they need the originals.
- `NewRequestID` generates a new UUID v4 on each call. IDs are unique within practical limits but not guaranteed globally unique.

## Dependencies

| Package | Used for |
|---------|----------|
| `bytes` | `bytes.Buffer` for `LimitedBuffer` |
| `io` | `io.ReadCloser`, `io.Writer` interfaces |
| `net/http` | `http.Header`, `http.Handler`, `http.Server` |
| `time` | `time.Duration` for server timeouts |
| `github.com/google/uuid` | UUID v4 generation for request IDs |

## Edge Cases

- **Buffer at exactly the limit**: `Write(p)` with `remaining == 0` sets `truncated = true` and does not write. Returns `len(p), nil`.
- **Buffer below zero**: `limit <= 0` disables truncation entirely. All writes go through.
- **Empty write**: `Write([]byte{})` returns `0, nil` without triggering truncation.
- **Tee with nil writer**: Returns the original reader — no mirroring occurs. Close is still delegated.
- **Tee with nil reader**: Returns `nil` — read attempts will panic (caller responsibility to avoid).
- **Request ID collision**: UUID v4 collision probability is negligible for practical use. 128 bits of randomness.

## Related

- [`../domains/traffic-capture/mitm.md`](../domains/traffic-capture/mitm.md) — MITM proxy consumes `LimitedBuffer`, `TeeReadCloser`, `RemoveHopByHop`, `NewRequestID`
- [`../architecture/layers.md`](../architecture/layers.md) — utility layer classification
