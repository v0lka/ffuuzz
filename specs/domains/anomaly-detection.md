# Anomaly Detection

## Overview

Detects abnormal HTTP responses during fuzzing. Four pluggable detectors are composed via `MultiDetector`, each checking a different class of anomaly (timeout, server error, latency regression, regex match on body). Detection is stateless per exchange — each mutated request/response is evaluated independently.

## Key Files

| File | Role |
|------|------|
| `internal/anomaly/detector.go` | `Detector` interface, `AnomalyHit`/`BaselineEntry` types, 4 detector implementations, `MultiDetector` |

## Core Types

```go
// AnomalyHit represents a detected anomaly from a single exchange.
type AnomalyHit struct {
    Type       model.FindingType
    Method     string
    Endpoint   string
    Details    model.FindingDetails
    Exchange   model.Exchange
    ResultBody []byte
}

// BaselineEntry holds per-endpoint baseline data.
type BaselineEntry struct {
    Method     string
    Endpoint   string
    P50Ms      int64
    StatusCode int
}

// Detector checks a replay result against baseline for anomalies.
type Detector interface {
    Detect(ex model.Exchange, result replayer.ExchangeResult,
           baseline *BaselineEntry, cfg model.AnomalyConfig) []AnomalyHit
}
```

## Detectors

### TimeoutDetector
Flags exchanges that exceeded the request timeout. Checks `result.Err` for network timeout errors (including `os.ErrDeadlineExceeded`). Produces `FindingTimeout` hits.

### ServerErrorDetector
Flags exchanges that returned HTTP 5xx status codes. Respects `cfg.Detect5xx` flag. Skips results where the baseline was already a 5xx (to avoid flagging endpoints that always 500). Produces `FindingServerError` hits.

### LatencyDetector
Flags exchanges where duration exceeded `baseline.P50Ms * cfg.LatencyMultiplier`. Requires both a non-nil baseline and a non-zero multiplier. Produces `FindingLatencyRegression` hits.

### RegexDetector
Flags exchanges where the response body matches any pattern in `cfg.RegexPatterns`. Performs regex matching on the decoded response body string. Produces `FindingRegexMatch` hits.

### MultiDetector
Created via `NewMultiDetector(cfg, logger)`. Composes all four detectors. `Detect()` calls each detector in sequence and concatenates results:

```go
func NewMultiDetector(cfg model.AnomalyConfig, logger zerolog.Logger) Detector {
    detectors := []Detector{
        &TimeoutDetector{},
    }
    if cfg.Detect5xx {
        detectors = append(detectors, &ServerErrorDetector{})
    }
    if cfg.LatencyMultiplier > 0 {
        detectors = append(detectors, &LatencyDetector{})
    }
    if len(cfg.RegexPatterns) > 0 {
        detectors = append(detectors, &RegexDetector{patterns: compilePatterns(cfg.RegexPatterns)})
    }
    return &multiDetector{detectors: detectors, logger: logger}
}
```

With a zero-value `AnomalyConfig`, only `TimeoutDetector` is active. This ensures the engine always has at least one detector.

## Flow

```
Worker calls detector.Detect(ex, result, baseline, cfg)
    │
    ▼
MultiDetector iterates over active detectors:
    │
    ├── TimeoutDetector.Detect()
    │   └─ isTimeoutError(result.Err)?
    │       ├─ YES → []AnomalyHit{FindingTimeout}
    │       └─ NO  → nil
    │
    ├── ServerErrorDetector.Detect()  (if Detect5xx enabled)
    │   └─ result.StatusCode >= 500 && baseline not 5xx?
    │       ├─ YES → []AnomalyHit{FindingServerError}
    │       └─ NO  → nil
    │
    ├── LatencyDetector.Detect()  (if LatencyMultiplier > 0)
    │   └─ result.DurationMs > baseline.P50Ms * multiplier?
    │       ├─ YES → []AnomalyHit{FindingLatencyRegression}
    │       └─ NO  → nil
    │
    └── RegexDetector.Detect()  (if patterns configured)
        └─ any regex matches response body?
            ├─ YES → []AnomalyHit{FindingRegexMatch} (per match)
            └─ NO  → nil

    → concatenated []AnomalyHit returned to Worker
```

## Invariants

- Every `MultiDetector` always includes `TimeoutDetector`. It is the only detector that cannot be disabled.
- `ServerErrorDetector` skips hits when the baseline status was also ≥500. This prevents flagging endpoints that normally return 5xx.
- `LatencyDetector` requires both a non-nil `BaselineEntry` and `LatencyMultiplier > 0`. Without either, it is not composed into `MultiDetector`.
- `RegexDetector` compiles patterns once in `NewMultiDetector`. Bad patterns log a warning and are skipped at startup, not at detection time.
- Detection is always called per-exchange, not batched. Each `Detect()` call receives exactly one `ExchangeResult`.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/model` | `FindingType`, `FindingDetails`, `AnomalyConfig`, `Exchange` |
| `internal/replayer` | `ExchangeResult` (replay output type) |

## Edge Cases

- **Replay error (non-timeout)**: Only `TimeoutDetector` checks `result.Err`. Other detectors return nil for errored results.
- **Nil baseline**: `LatencyDetector` skips. `ServerErrorDetector` treats nil baseline as "no previous 5xx" (will flag 5xx responses).
- **Empty regex patterns**: `RegexDetector` is not composed into `MultiDetector`, so its `Detect()` is never called.
- **Truncated body**: Regex matching works on the truncated body. The detector does not know or care that the body was truncated.

## Configuration

| Config field | Type | Default | Effect |
|---|---|---|---|
| `AnomalyConfig.Detect5xx` | bool | false | Enables `ServerErrorDetector` |
| `AnomalyConfig.LatencyMultiplier` | float64 | 0 | Enables `LatencyDetector` with multiplier threshold |
| `AnomalyConfig.RegexPatterns` | []string | nil | Enables `RegexDetector` with these patterns |
