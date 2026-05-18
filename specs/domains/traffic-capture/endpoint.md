# Endpoint Resolution

## Responsibility

Normalizes HTTP paths (replacing dynamic segments like UUIDs and numeric IDs with `{_}`) and detects parameterized endpoint patterns via statistical trie analysis. When enough recordings share a parameterized path segment, the resolver triggers a database merge that collapses those recordings into a single endpoint group.

## Key Types

```go
// Origin identifies a target by scheme, host, and port.
type Origin struct {
    Scheme string
    Host   string
    Port   int
}

// Merger is the interface that the DB layer implements for recording merge
// operations triggered by endpoint pattern collapses.
type Merger interface {
    MergeRecordings(ctx context.Context, origin Origin,
        sourcePrefixes []string, targetPrefix string) (int, error)
    ListDistinctPaths(ctx context.Context, origin Origin) ([]string, error)
    ListOrigins(ctx context.Context) ([]Origin, error)
}

type Resolver struct {
    mu     sync.Mutex
    tries  map[Origin]*trieNode
    merger Merger
    logger zerolog.Logger
}
```

## Public API

### `NewResolver(merger Merger, logger zerolog.Logger) *Resolver`
Creates an empty resolver. Call `RebuildFromDB` to populate the trie from existing recordings.

### `RebuildFromDB(ctx context.Context) error`
Queries all origins and their distinct paths via the `Merger` interface, then inserts every path into the trie. Called once at startup. If it fails, the proxy starts with an empty trie and builds it incrementally.

### `ObservePath(origin Origin, normalizedPath string) string`
Records an already-normalised path for the given origin. Updates the trie. If the observation triggers a statistical collapse (a segment has exceeded the cardinality threshold), fires an async `MergeRecordings` call and returns the collapsed path. Otherwise returns the input path unchanged.

### `NormalizePath(path string) string`
Standalone heuristic function. Applies regex-based normalisation:
- UUIDs (`/[0-9a-fA-F]{8}-...`) → `/{uuid}`
- Numeric IDs (`/\d+`) → `/{id}`
- ISO timestamps → `/{datetime}`
- GitHub-style base58 IDs → `/{id}`

Returns the path with dynamic segments replaced. This is called by the recorder before `ObservePath`.

### `SplitPathSegments(path string) []string`
Splits a path into segments, preserving leading/trailing slashes and handling edge cases (root path, double slashes).

## Two-Phase Normalization

### Phase 1: Heuristic (Regex-based)
`NormalizePath()` uses regex patterns to identify well-known parameter types. This is cheap, synchronous, and catches obvious cases without statistical analysis.

### Phase 2: Statistical (Trie-based)
The resolver maintains a segment trie per origin. Each node tracks how many distinct values have been observed at that segment position. When the distinct value count exceeds a threshold (default 5 within a configurable window), the segment is collapsed to `{_}` and all recordings matching that prefix are merged in the database.

## Trie Structure

```
Origin: https://api.example.com:443

Trie:
  /
  └── api/
      └── v1/
          ├── users/     ← collapsed to {_} after 5+ distinct values observed
          │   └── ...
          ├── orders/    ← collapsed to {_}
          │   └── ...
          └── health     ← leaf, low cardinality, stays as "health"
```

Each `trieNode` tracks:
- `segment`: the path segment literal
- `children`: map of child segments → child nodes
- `valueSet`: set of distinct values seen at this position (for collapse detection)
- `collapsed`: whether this node has been collapsed to `{_}`

## Collapse Detection

When `ObservePath` is called:
1. Walk the trie, inserting segments
2. At each level, add the segment value to `valueSet`
3. If `len(valueSet) >= threshold` and not yet collapsed:
   - Mark node as collapsed
   - Fire async goroutine: `merger.MergeRecordings(ctx, origin, sourcePrefixes, targetPattern)`
   - The merge rewrites `target_path` for all recordings matching the source prefixes
   - Returns the collapsed path pattern

## Invariants

- `NormalizePath()` is called before `ObservePath()`. The heuristic phase always runs first.
- Path normalisation is idempotent. Calling `NormalizePath` twice produces the same result.
- The trie is per-origin. A path like `/api/users/123` on `https://api.example.com:443` is tracked independently from the same path on `https://api.example.com:8443`.
- `ObservePath` accepts only normalized paths as input. Passing a raw path may prevent proper collapse detection.
- Merge operations are asynchronous. The recording is already persisted before the merge fires. A failed merge does not affect the recording.
- `RebuildFromDB` errors are non-fatal. The resolver starts with an empty trie and rebuilds as traffic arrives.
- The resolver's mutex protects all trie operations. `ObservePath` and `RebuildFromDB` cannot run concurrently.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/metrics` | `EndpointCollapses` counter, `EndpointMerges` counter |

## Edge Cases

- **Root path (`/`)**: Returns `"/"`. Single segment.
- **Empty path**: Returns `""`.
- **Path with trailing slash**: Preserved in normalization. `/users/123/` → `/users/{id}/`.
- **All segments already collapsed**: `ObservePath` returns the path unchanged.
- **Merge times out**: The merge goroutine has a 60-second deadline. On timeout, it logs a warning and the collapse retries on the next observation.
