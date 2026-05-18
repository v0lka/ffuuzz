# Engine → Replayer

## Overview

The engine uses the `Replayer` for replaying mutated exchanges against the target and the `SessionReplayer` interface (defined in `internal/triage`) for replaying full sessions during triage. Both are instantiated inside `runCampaign()`.

## Interfaces

```go
// internal/replayer/replayer.go

type Replayer struct {
    client *http.Client
    logger zerolog.Logger
}

// internal/triage/triage.go

type SessionReplayer interface {
    ReplaySession(ctx context.Context, session model.RecordingSession,
        baseURL string, wctx *replayer.WorkerContext,
        logger zerolog.Logger) ([]replayer.ExchangeResult, error)
}
```

The `SessionReplayer` interface is defined in `internal/triage` (consumer owns the interface) and implemented by `Replayer`. This enables testability — triage tests can inject mock session replayers.

## Implementations

| Interface | Implementation | Notes |
|-----------|---------------|-------|
| (concrete) | `replayer.Replayer` | `ReplayExchange`, `ReplaySession` |
| `triage.SessionReplayer` | `replayer.Replayer` | Via `ReplaySession` method, used for triage confirmation and minimization |

## Initialization

Inside `runCampaign()` in `internal/engine/engine.go`:

```go
rep := replayer.New(nil, logger)  // nil client → creates default HTTP client

worker := NewWorker(WorkerConfig{
    Replayer:   rep,
    // ...
})

triager := triage.NewTriager()
// Replayer is passed to triager via Confirm/MinimizeSession/MinimizeJSONBody params
```

## Operations

### Engine → Replayer (per-exchange)

```go
// worker.go
result := w.Replayer.ReplayExchange(ctx, mutatedExchange, baseURL, wctx)
```

Called by each worker for every mutated exchange. The `WorkerContext` (`wctx`) carries per-worker state (cookies, variables).

### Triage → Replayer (per-session)

```go
// triage.go
results, err := rep.ReplaySession(ctx, session, baseURL, wctx, nil)
```

Called during confirmation and minimization. Creates a fresh `WorkerContext` for each check (`stillTriggers`). This ensures state isolation between confirmation attempts.

## Data Flow

```
Worker:
    │
    ├── pipeline.Mutate(exchange, rng) → mutatedExchange
    │
    ├── rep.ReplayExchange(ctx, mutatedExchange, baseURL, wctx) → ExchangeResult
    │   │
    │   ├── Apply variable substitutions from wctx.Variables
    │   ├── Update wctx.CookieJar with Set-Cookie headers
    │   ├── Extract variables from response → wctx.Variables
    │   └── Return ExchangeResult{StatusCode, RespBody, DurationMs, Err}
    │
    └── detector.Detect(exchange, result, baseline, cfg) → []AnomalyHit

Triage (Confirm/Minimize):
    │
    ├── Create fresh WorkerContext (new cookies, empty variables)
    ├── rep.ReplaySession(ctx, session, baseURL, wctx, nil) → []ExchangeResult
    │   └── Replays all exchanges sequentially, accumulating state
    │
    └── Check if detector fires on any ExchangeResult
```

## Breaking Change Checklist

- [ ] Is `ReplayExchange` signature changing? → update all callers in `internal/engine/worker.go`
- [ ] Is `ReplaySession` signature changing? → update all callers in `internal/triage/triage.go`
- [ ] Is `SessionReplayer` interface changing? → update `Replayer` implementation and all triage tests
- [ ] Is `ExchangeResult` struct changing? → update anomaly detectors, triage, and worker
- [ ] Is `WorkerContext` structure changing? → update `NewWorkerContext` callers in engine and triage
- [ ] Are variable substitution semantics changing? → verify engine extraction rule conversion and triage isolation

## Related

- [`domains/fuzzing-engine/replayer.md`](../domains/fuzzing-engine/replayer.md) — replayer implementation details
- [`domains/fuzzing-engine/engine.md`](../domains/fuzzing-engine/engine.md) — worker loop
- [`domains/triage.md`](../domains/triage.md) — triage confirmation and minimization
- [`engine-stores.md`](engine-stores.md) — Engine → DB stores
