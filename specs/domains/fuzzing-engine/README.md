# Fuzzing Engine

## Overview

The fuzzing engine orchestrates the core "fuzz" phase: loading recording seeds, spawning worker pools that mutate and replay exchanges, detecting anomalies, and triaging findings. A background reproduce worker independently validates and persists confirmed findings. The engine owns the campaign lifecycle from `STARTING` through `FINISHED`/`STOPPED`/`FAILED`.

## Key Files

| File | Role |
|------|------|
| `internal/engine/engine.go` | `Engine` struct, `StartCampaign`, `StopCampaign`, `StopAll`, `runCampaign`, epsilon-greedy seed selection |
| `internal/engine/worker.go` | `Worker` struct, fuzz loop (mutate → replay → detect → triage → persist), intensity/feedback tracking |
| `internal/engine/intensity.go` | `IntensityTracker`: per-operator productivity statistics, adaptive intensity multipliers |
| `internal/engine/feedback.go` | `SeedInterestTracker`: coverage-guided seed scoring, weighted selection weights |
| `internal/engine/limiter.go` | `Limiter`: token-bucket rate limiting |
| `internal/engine/reproduce.go` | `ReproduceWorker`: background finding reproduction |
| `internal/mutate/mutate.go` | `Pipeline`: mutation pipeline orchestrator |
| `internal/mutate/dictionary.go` | `Dictionary`: user-supplied header values with per-endpoint support, traffic extraction |
| `internal/replayer/replayer.go` | `Replayer`: HTTP replay client |
| `internal/corpus/manager.go` | `Manager`: seed loading, P50 baseline computation |

## Core Types

```go
type Engine struct {
    campaigns   CampaignStore
    findings    FindingStore
    artifacts   ArtifactStore
    corpus      *corpus.Manager
    artifactDir string
    logger      zerolog.Logger
    mu          sync.Mutex
    running     map[string]context.CancelFunc
    reproduceCancel context.CancelFunc
    reproduceWG     sync.WaitGroup
}
```

## Flow

```
API: POST /campaigns/:id/start
    │
    ▼
Engine.StartCampaign(campaign)
    │
    ├── CREATED → STARTING (optimistic lock)
    │
    ├── corpus.GetSeeds(campaignID) → []RecordingSession
    │   └─ if empty → FAILED
    │
    ├── corpus.ComputeBaseline(seeds) → map[endpoint]P50
    │
    ├── STARTING → RUNNING
    │
    └── go runCampaign(ctx, campaignID, cfg, seeds, baselines)
            │
            ├── Create mutation Pipeline
            ├── pipeline.Dict().ExtractFromTraffic(seeds)
            │
            ├── Create IntensityTracker → pipeline.SetIntensityCallback(...)
            ├── Create SeedInterestTracker
            │
            ├── Create anomaly MultiDetector
            ├── Create Triager
            ├── Create Replayer
            ├── Create Limiter (token bucket)
            │
            ├── Spawn N workers → consume from taskCh
            │   │
            │   └── Worker loop: for each SeedTask:
            │       1. pipeline.Mutate(exchange, rng)
            │       2. intensityTracker.RecordApplication(ops)
            │       3. replayer.ReplayExchange(...)
            │       4. feedbackTracker.RecordResponse(seedID, status, errorBody)
            │       5. detector.Detect(...)
            │       6. for each AnomalyHit:
            │            triage.Signature → dedup
            │            triage.Confirm → N-replay
            │            triage.MinimizeSession
            │            triage.MinimizeJSONBody
            │            intensityTracker.RecordFinding(ops)
            │            feedbackTracker.RecordFinding(seedID)
            │            persist Finding + Artifact to DB
            │
            └── Task generator (single goroutine):
                │  loop until ctx.Done() || max_tests || duration_sec
                │  limiter.Acquire() (blocks if over RPS)
                │  seed selection:
                │    80%: weightedPick (exploitation — interest-weighted)
                │    20%: uniform random (exploration)
                │  taskCh ← SeedTask
                │
                ├── max_tests reached → FINISHED
                ├── duration_sec reached → FINISHED
                └── ctx.Done() → STOPPED
```

## Components

- [`engine.md`](engine.md) — `Engine`: campaign lifecycle, rate limiting, reproduce worker
- [`mutate.md`](mutate.md) — Mutation pipeline: URI, header, JSON, param, sequence, primitive mutators
- [`replayer.md`](replayer.md) — HTTP replay client with stateful `WorkerContext`
- [`corpus.md`](corpus.md) — Seed loading and P50 baseline computation

## Invariants

- `StartCampaign` transitions status synchronously (CREATED → STARTING → RUNNING). The fuzz loop (`runCampaign`) runs asynchronously.
- Seed loading is synchronous. If no seeds exist, the campaign transitions to FAILED before any worker goroutine starts.
- Each worker has its own `WorkerContext` (cookies, variables). State is not shared between workers.
- The task generator is a single goroutine. N workers consume from a shared buffered channel.
- Rate limiting (token bucket) is applied before the task generator pushes to the channel, not inside workers.
- The reproduce worker is a single goroutine started at application startup. It polls for enqueued reproduce jobs.
- All status transitions use optimistic locking via `CampaignStore.UpdateStatus(ctx, id, oldStatus, newStatus) (bool, error)`.

## Configuration

| Campaign config field | Type | Default | Effect |
|---|---|---|---|
| `Limits.Workers` | int | 0 (default 4) | Number of parallel fuzzing workers |
| `Limits.RPS` | int | 50 | Maximum requests per second (token bucket) |
| `Limits.MaxTests` | int | 0 (unlimited) | Maximum number of mutations to generate |
| `Limits.DurationSec` | int | 0 (unlimited) | Maximum campaign duration in seconds |
| `Limits.ReqTimeoutMs` | int64 | 3000 | Timeout per replayed request |
