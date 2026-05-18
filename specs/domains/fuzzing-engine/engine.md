# Engine

## Responsibility

Orchestrates fuzzing campaigns: manages the campaign lifecycle, spawns and manages worker pools with rate limiting, and runs a background reproduce worker for finding validation.

## Key Types

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

// CampaignStore defines the campaign operations needed by the engine.
type CampaignStore interface {
    UpdateStatus(ctx context.Context, id string,
        oldStatus, newStatus model.CampaignStatus) (bool, error)
    IncrementStats(ctx context.Context, id string,
        testsDelta, findingsDelta int) error
}

// FindingStore defines the finding operations needed by the engine.
type FindingStore interface {
    ExistsBySignature(ctx context.Context, campaignID, signature string) (bool, error)
    Create(ctx context.Context, f model.Finding) error
    UpdateStatus(ctx context.Context, id string, status model.FindingStatus) error
    GetByID(ctx context.Context, id string) (*model.Finding, error)
    ClaimNextReproduceJob(ctx context.Context) (string, int, bool, error)
    SetReproduceStatus(ctx context.Context, id, status string) error
}

// ArtifactStore defines the artifact operations needed by the engine.
type ArtifactStore interface {
    Create(ctx context.Context, a model.Artifact) error
    GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error)
}
```

## Public API

### `NewEngine(campaigns, findings, artifacts, corpus, artifactDir, logger) *Engine`
Creates the engine with all required store dependencies and the corpus manager.

### `StartCampaign(ctx context.Context, campaign *model.Campaign) error`
Starts a fuzzing campaign. Transitions CREATED → STARTING → RUNNING. Loads seeds and computes baselines synchronously, then spawns a goroutine (`runCampaign`) for the fuzz loop. Returns an error if seed loading fails, no seeds exist, or status transitions fail.

### `StopCampaign(ctx context.Context, id string) error`
Cancels a running campaign. Transitions RUNNING → STOPPING. Cancels the campaign context, which stops the task generator and all workers. The campaign goroutine transitions to STOPPED after workers drain.

### `StopAll(ctx context.Context)`
Cancels all running campaigns and the reproduce worker. Used during graceful shutdown. Transitions each campaign RUNNING → STOPPING.

### `IsRunning(id string) bool`
Thread-safe check whether a campaign is currently managed by the engine.

### `StartReproduceWorker(ctx context.Context)`
Spawns the background reproduce-finding polling goroutine. Called once at application startup.

## Internal Flow

### runCampaign (goroutine)

```
1. Create mutation pipeline from config
2. Create anomaly MultiDetector
3. Create Triager
4. Create Replayer
5. Convert extraction rules
6. Create Limiter (token bucket)
7. Create task channel (WorkerCount * 2 capacity)
8. Spawn Workers → each runs w.Run(ctx, taskCh)
9. Task generator loop:
   └─ for each iteration:
      ├── check ctx.Done()
      ├── check max_tests
      ├── check duration_sec deadline
      ├── limiter.Acquire(ctx) → blocks if over RPS
      ├── pick random seed session
      ├── generate random mutation seed
      └── taskCh ← SeedTask
10. close(taskCh) → workers detect closed channel → exit
11. wg.Wait() → all workers done
12. UpdateStatus → FINISHED (or STOPPED if ctx cancelled)
```

### Worker Loop

```
for task := range taskCh:
    │
    ├── Create per-task rng from task.MutationSeed
    │
    ├── Apply sequence mutations (if enabled):
    │   └── seqMutator.Mutate(task.Session.Entries, rng, intensity)
    │
    ├── For each (possibly sequence-mutated) exchange:
    │   │
    │   ├── pipeline.Mutate(exchange, rng) → MutationResult
    │   │
    │   ├── replayer.ReplayExchange(result.Exchange, baseURL, wctx) → ExchangeResult
    │   │
    │   ├── detector.Detect(exchange, result, baseline, anomalyCfg) → []AnomalyHit
    │   │
    │   └── For each hit → triage + persist
    │
    └── campaigns.IncrementStats(campaignID, 1, findingsDelta)
```

## Rate Limiting

`Limiter` is a token-bucket implementation:

```go
type Limiter struct { ... }
func NewLimiter(rps int) *Limiter
func (l *Limiter) Acquire(ctx context.Context) error
func (l *Limiter) Close()
```

- Refills at `rps` tokens per second
- `Acquire()` blocks until a token is available or the context is cancelled
- Called in the task generator, not in workers
- When rps=0, `Acquire()` returns immediately (unlimited rate)

## Reproduce Worker

`ReproduceWorker` polls for enqueued reproduce jobs:

```go
type ReproduceWorker struct { ... }
func NewReproduceWorker(findings, artifacts, artifactDir, logger) *ReproduceWorker
func (w *ReproduceWorker) Run(ctx context.Context)
```

1. Poll `findings.ClaimNextReproduceJob(ctx)` — atomically claims the oldest pending job
2. Load the artifact JSON from disk
3. Replay the finding N times against the target
4. Set reproduce status: CONFIRMED if anomaly reproduced, NOT_REPRODUCED if not
5. Loop until context cancelled or no more jobs

## Invariants

- The engine tracks running campaigns via `running map[string]context.CancelFunc` protected by `sync.Mutex`. This map is the authoritative source for whether a campaign is running.
- `StartCampaign` uses `context.Background()` for the campaign context, decoupling the campaign lifecycle from the HTTP request that started it. Ctx cancellation from the API caller does not affect the campaign.
- `StopAll` stops the reproduce worker before stopping campaigns. This prevents new reproduce jobs from being claimed during shutdown.
- When a campaign finishes, the engine tries `RUNNING → FINISHED`, then `STOPPING → FINISHED` as fallback (if `StopCampaign` was called during the fuzz loop).
- Campaign context cancellation is the primary stop mechanism. Workers check `ctx.Done()` in their loop and between operations.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/anomaly` | `Detector`, `MultiDetector`, `BaselineEntry`, `AnomalyHit` |
| `internal/corpus` | `Manager` for seed loading and baseline computation |
| `internal/model` | `Campaign`, `CampaignConfig`, `CampaignStatus`, `Finding`, `Artifact` |
| `internal/mutate` | `Pipeline`, `SeqMutator`, `Config` |
| `internal/replayer` | `Replayer`, `ExchangeResult`, `WorkerContext`, `ExtractionRule` |
| `internal/triage` | `Triager` for dedup, confirm, minimize |
| `internal/metrics` | `TestsTotal`, `FindingsTotal` |

## Edge Cases

- **No seeds for campaign**: `StartCampaign` returns error "no seeds", campaign transitions to FAILED.
- **RPS = 0**: Limiter's `Acquire()` returns immediately — no rate limiting.
- **Worker count not set**: Defaults to 4 workers.
- **MaxTests and DurationSec both 0**: Campaign runs indefinitely until manually stopped.
- **Reproduce worker encounters corrupt artifact**: Logs error and marks reproduce status as FAILED. Continues polling.
- **Duplicate findings**: `ExistsBySignature` check prevents duplicate `Create` calls. Two workers detecting the same anomaly on different exchanges produce only one finding.
