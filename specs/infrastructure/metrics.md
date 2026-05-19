# Prometheus Metrics

## Responsibility

Defines and registers all Prometheus metrics for the ffuuzz proxy and engine. Uses a custom (non-global) `prometheus.Registry` to avoid polluting the default registry. All metrics are registered at package `init()` time via a panic-on-duplicate strategy.

## Key Files

| File | Role |
|------|------|
| `internal/metrics/metrics.go` | All metric definitions, `init()` registration, `Registry()` accessor |

## Core Types

```go
// Registry returns the custom Prometheus registry with all ffuuzz metrics.
func Registry() *prometheus.Registry
```

There is no exported struct — the registry is a package-level variable exposed via `Registry()`.

## Metric Catalog (11 total)

### Engine Metrics

| Variable | Type | Name | Labels | Description |
|----------|------|------|--------|-------------|
| `TestsTotal` | Counter | `ffuuzz_tests_total` | — | Total number of fuzz tests executed |
| `FindingsTotal` | CounterVec | `ffuuzz_findings_total` | `type` | Total number of findings by anomaly type |
| `RequestDuration` | Histogram | `ffuuzz_request_duration_seconds` | — | Histogram of upstream request durations in seconds |
| `CorpusSize` | Gauge | `ffuuzz_corpus_size` | — | Current number of recording sessions in the corpus |

### Certificate Cache Metrics

| Variable | Type | Name | Labels | Description |
|----------|------|------|--------|-------------|
| `CertCacheHits` | Counter | `ffuuzz_cert_cache_hits_total` | — | Certificate LRU cache hits |
| `CertCacheMisses` | Counter | `ffuuzz_cert_cache_misses_total` | — | Certificate LRU cache misses |
| `CertCacheEvictions` | Counter | `ffuuzz_cert_cache_evictions_total` | — | Certificate LRU cache evictions |

### Error Metrics

| Variable | Type | Name | Labels | Description |
|----------|------|------|--------|-------------|
| `ConnectErrors` | CounterVec | `ffuuzz_connect_errors_total` | `error_class` | CONNECT/hijack errors by error class |
| `CertErrors` | Counter | `ffuuzz_cert_errors_total` | — | Certificate generation or storage errors |

### Endpoint Metrics

| Variable | Type | Name | Labels | Description |
|----------|------|------|--------|-------------|
| `EndpointCollapses` | Counter | `ffuuzz_endpoint_collapses_total` | — | Total endpoint pattern collapses detected |
| `EndpointMerges` | Counter | `ffuuzz_endpoint_merges_total` | — | Total recording merges from endpoint collapses |

## Public API

### `Registry() *prometheus.Registry`

Returns the custom `prometheus.Registry` containing all ffuuzz metrics. This is used by the API server to expose metrics at `/metrics` without exposing the Go runtime's default metrics.

## Initialization

All 11 metrics are registered in a package-level `init()` function:

```go
func init() {
    collectors := []prometheus.Collector{
        TestsTotal, FindingsTotal, RequestDuration, CorpusSize,
        CertCacheHits, CertCacheMisses, CertCacheEvictions,
        ConnectErrors, CertErrors, EndpointCollapses, EndpointMerges,
    }
    for _, c := range collectors {
        if err := reg.Register(c); err != nil {
            panic("failed to register Prometheus metric: " + err.Error())
        }
    }
}
```

## Configuration

**Bucket configuration**: `RequestDuration` uses `prometheus.DefBuckets` (default buckets: 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s).

**Label values for `FindingsTotal.type`**: Populated at runtime with `model.FindingType` values (TIMEOUT, SERVER_ERROR, LATENCY_REGRESSION, REGEX_MATCH).

**Label values for `ConnectErrors.error_class`**: Set at runtime by the MITM proxy for CONNECT/hijack failures. Values include error type strings from the proxy's error handling.

## Invariants

- The custom registry is NOT the global default. `prometheus.DefaultRegisterer` is not used.
- Duplicate metric registration panics at `init()` time (fail-fast). This catches metric name collisions at startup, not at runtime.
- All metrics use `ffuuzz_` prefix to namespace them within a shared Prometheus deployment.
- `FindingsTotal` and `ConnectErrors` are `CounterVec` with label dimensions — label values are set at increment time.
- Metrics are never unregistered. They live for the lifetime of the process.

## Dependencies

| Package | Used for |
|---------|----------|
| `github.com/prometheus/client_golang/prometheus` | Counter, CounterVec, Histogram, Gauge, Registry |

## Related

- [`config.md`](config.md) — application configuration that drives metric collection behavior
- [`../architecture/security-model.md`](../architecture/security-model.md) — certificate metrics usage in CertStore
- [`../architecture/layers.md`](../architecture/layers.md) — infrastructure layer classification
