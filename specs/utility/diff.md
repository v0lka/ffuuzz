# Transaction Diffing

## Responsibility

Computes structural differences between two recorded HTTP transactions (`TxRecord` from `internal/recorder`). Currently compares URL and response status code. Designed for extensibility — additional fields can be added as needed.

## Key Files

| File | Role |
|------|------|
| `internal/diff/diff.go` | `FieldDiff`, `TxDiff`, `DiffTxRecords` |

## Core Types

```go
// FieldDiff describes a single field-level difference between two recorded transactions.
type FieldDiff struct {
    Field string `json:"field"` // "url" or "resp_status"
    Old   any    `json:"old"`
    New   any    `json:"new"`
}

// TxDiff describes the differences between two recorded transactions.
type TxDiff struct {
    RequestIDA string      `json:"request_id_a"`
    RequestIDB string      `json:"request_id_b"`
    Diffs      []FieldDiff `json:"diffs"`
}
```

## Public API

### `DiffTxRecords(a, b recorder.TxRecord) TxDiff`

Computes the structural differences between two recorded transactions. Compares:

1. **URL** — string equality (`a.URL != b.URL`)
2. **Response status code** — integer equality (`a.RespStatus != b.RespStatus`)

Returns a `TxDiff` with `Diffs` containing one `FieldDiff` per differing field. Returns `Diffs: nil` when both transactions are identical.

## Internal Flow

```
DiffTxRecords(a, b):
    │
    ├── RequestIDA = a.RequestID
    ├── RequestIDB = b.RequestID
    │
    ├── a.URL != b.URL ?
    │   └── YES → append FieldDiff{Field: "url", Old: a.URL, New: b.URL}
    │
    ├── a.RespStatus != b.RespStatus ?
    │   └── YES → append FieldDiff{Field: "resp_status", Old: a.RespStatus, New: b.RespStatus}
    │
    └── Return TxDiff with collected diffs
```

## Compared Fields

| Field | JSON key | Type | Comparison |
|-------|----------|------|------------|
| URL | `"url"` | `string` | String equality (`!=`) |
| Response status | `"resp_status"` | `int` | Integer equality (`!=`) |

## Extensibility

Additional fields can be added to `DiffTxRecords` by appending to `res.Diffs` when a difference is detected. Potential additions:

- **HTTP method**: `a.Method != b.Method`
- **Request headers**: deep comparison of `ReqHeaders` maps
- **Response headers**: deep comparison of `RespHeaders` maps
- **Duration**: `a.Timings["duration_ms"] != b.Timings["duration_ms"]`
- **Body truncation flags**: `a.ReqTrunc != b.ReqTrunc` or `a.RespTrunc != b.RespTrunc`

Each new comparison should use a consistent field name (snake_case for JSON output).

## Invariants

- `DiffTxRecords` is a pure function. No I/O, no side effects. Output depends only on inputs.
- `Diffs` is nil (not empty slice) when no differences are found. JSON serialization produces `null`, not `[]`.
- `FieldDiff.Old` and `FieldDiff.New` are typed as `any` to accommodate heterogeneous field types (string URLs, int status codes).
- Only top-level fields are compared. No recursive comparison of nested structures.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/recorder` | `TxRecord` type for input |

## Edge Cases

- **Identical transactions**: Returns `TxDiff{Diffs: nil}` (no differences).
- **Empty URL**: Compared as empty string `""`. If both are empty, no diff.
- **Zero status code**: Compared as `0`. Diff only if one status is zero and the other is non-zero.

## Related

- [`../domains/traffic-capture/recorder.md`](../domains/traffic-capture/recorder.md) — `TxRecord` type definition
- [`../architecture/layers.md`](../architecture/layers.md) — utility layer classification
