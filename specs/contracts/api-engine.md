# API → Engine

## Overview

The API server delegates campaign lifecycle operations and reproduce requests to the `engine.Engine`. The API owns the REST endpoints; the engine owns the business logic. The API passes the `engine.Engine` concrete type (not an interface) since there is only one implementation.

## Interfaces

The API does not define an interface for the engine — it references `*engine.Engine` directly:

```go
// internal/api/server.go

type Server struct {
    // ...
    engine *engine.Engine
}
```

This is acceptable because:

- There is exactly one engine implementation
- The engine is a domain orchestration component, not an infrastructure adapter
- The API and engine are in different layers (boundary vs domain), but within the same codebase

## Implementations

| Package           | Type     | Notes                                        |
| ----------------- | -------- | -------------------------------------------- |
| `internal/engine` | `Engine` | Sole implementation, wired in `cli/serve.go` |

## Initialization

In `internal/cli/serve.go`:

```go
eng := engine.NewEngine(campaignStore, findingStore, artifactStore, corpusMgr, cfg.ArtifactDir, logger)
// ...
apiSrv := api.NewServer(api.ServerConfig{
    Engine: eng,
    // ...
})
```

## Operations Exposed to API

| API handler        | Engine method                      | Description                                                      |
| ------------------ | ---------------------------------- | ---------------------------------------------------------------- |
| `startCampaign`    | `eng.StartCampaign(ctx, campaign)` | Start fuzzing campaign                                           |
| `stopCampaign`     | `eng.StopCampaign(ctx, id)`        | Stop running campaign                                            |
| `getCampaignStats` | `eng.IsRunning(id)`                | Check if campaign is running                                     |
| `reproduceFinding` | (indirectly via finding store)     | Enqueues reproduce job                                           |
| `analyzeFinding`   | `s.llmTriager.AnalyzeFinding(...)` | Single-finding LLM analysis (synchronous, 200)                   |
| `analyzeCampaign`  | `s.llmTriager.BatchAnalyze(...)`   | Batch LLM analysis of unconfirmed findings (async, 202 Accepted) |

The `IsRunning` check is used in the stats endpoint to determine which stats field to include (real-time computed vs. stored).

## Data Flow

```
POST /api/v1/campaigns/:id/start
    │
    ├── api.startCampaign handler
    │   ├── Parse campaign ID
    │   ├── Load campaign from DB (via campaigns store)
    │   ├── Validate campaign status == CREATED
    │   ├── eng.StartCampaign(ctx, campaign)
    │   │   ├── CREATED → STARTING → RUNNING (status transitions)
    │   │   ├── Load seeds, compute baselines
    │   │   └── go runCampaign() (async fuzz loop)
    │   └── Return 200 OK (campaign is now RUNNING)
    │
POST /api/v1/campaigns/:id/stop
    │
    ├── api.stopCampaign handler
    │   ├── Parse campaign ID
    │   ├── eng.StopCampaign(ctx, id)
    │   │   ├── RUNNING → STOPPING (status transition)
    │   │   └── Cancel campaign context
    │   └── Return 200 OK
    │
POST /api/v1/findings/:id/reproduce
    │
    ├── api.reproduceFinding handler
    │   ├── Parse finding ID
    │   ├── findings.UpdateReproduceStatus(id, "ENQUEUED", 0)
    │   └── Return 202 Accepted
    │   (ReproduceWorker picks up the job asynchronously)
    │
POST /api/v1/findings/:id/analyze
    │
    ├── api.analyzeFinding handler
    │   ├── Parse finding ID
    │   ├── s.llmTriager == nil → 503 LLM_DISABLED
    │   ├── Load finding + artifact from DB
    │   ├── s.llmTriager.AnalyzeFinding(ctx, finding, artifactPayload)
    │   ├── findings.UpdateLLMAnalysis(ctx, id, analysisJSON)
    │   └── Return 200 OK with *LLMAnalysis JSON
    │
POST /api/v1/campaigns/:id/analyze
    │
    ├── api.analyzeCampaign handler
    │   ├── Parse campaign ID
    │   ├── s.llmTriager == nil → 503 LLM_DISABLED
    │   ├── List unconfirmed findings (limit 10000)
    │   ├── 0 findings → 200 OK {"analyzed": 0, "message": "..."}
    │   ├── Return 202 Accepted {"message": "analysis started", "total": N}
    │   └── Background goroutine:
    │       ├── context.WithTimeout(10 min)
    │       ├── s.llmTriager.BatchAnalyze(ctx, findings, artifactGetter, onResult)
    │       └── Each result persisted via findings.UpdateLLMAnalysis
    │
    │   The frontend polls GET /api/v1/campaigns/:id/findings for progress.
```

## Breaking Change Checklist

- [ ] Does the engine method signature change? → update all API handler callsites
- [ ] Does `StartCampaign` return a new error type? → update API error classification
- [ ] Does the engine need new fields in `ServerConfig`? → update `cli/serve.go` wiring
- [ ] Are there thread-safety implications? → engine uses `sync.Mutex` on `running` map

## Related

- [`engine-stores.md`](engine-stores.md) — Engine → DB stores boundary
- [`cli-infrastructure.md`](cli-infrastructure.md) — how the CLI wires engine and API together
