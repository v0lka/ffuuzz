# Engine

## Responsibility

Orchestrates fuzzing campaigns: manages the campaign lifecycle, spawns and manages worker pools with rate limiting, runs a background reproduce worker for finding validation, and performs vulnerability grouping on confirmed findings at campaign stop.

## Key Types

```go
type Engine struct {
    campaigns   CampaignStore
    findings    FindingStore
    artifacts   ArtifactStore
    corpus      *corpus.Manager
    llmTriager  *triage.LLMTriager
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
    UpdateLLMAnalysis(ctx context.Context, id string, analysisJSON []byte) error
}

// ArtifactStore defines the artifact operations needed by the engine.
type ArtifactStore interface {
    Create(ctx context.Context, a model.Artifact) error
    GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error)
}

// GroupingStore extends FindingStore with operations for vulnerability grouping.
// Consumed via type assertion; grouping is silently skipped if not implemented.
type GroupingStore interface {
    FindingStore
    ListAll(ctx context.Context, campaignID, typeFilter, statusFilter string,
        since *time.Time, limit, offset int) ([]model.Finding, error)
    UpdateFindingGroup(ctx context.Context, id, groupID string) error
}

// IntensityTracker maintains per-operator productivity statistics and computes
// adaptive intensity multipliers for campaign mutation. Safe for concurrent use.
type IntensityTracker struct {
    mu        sync.Mutex
    operators map[string]*operatorStats // "uri", "header", "json", "param", "primitive"
}

// SeedInterestTracker tracks per-seed response diversity for coverage-guided
// seed selection. Seeds producing novel status codes or error signatures receive
// higher interest, which translates to higher probability in weighted selection.
type SeedInterestTracker struct {
    mu     sync.Mutex
    scores map[string]*seedStats
}

// WorkerConfig bundles all dependencies for a Worker.
type WorkerConfig struct {
    ID               int
    CampaignID       string
    BaseURL          string
    Pipeline         *mutate.Pipeline
    SeqMutator       *mutate.SeqMutator
    Detector         *anomaly.MultiDetector
    Triager          *triage.Triager
    Replayer         *replayer.Replayer
    Findings         FindingStore
    Artifacts        ArtifactStore
    Campaigns        CampaignStore
    ArtifactDir      string
    AnomalyCfg       model.AnomalyConfig
    TriageCfg        model.TriageConfig
    Baselines        map[string]*anomaly.BaselineEntry
    ReqTimeoutMs     int64
    ExtractionRules  []replayer.ExtractionRule
    IntensityTracker *IntensityTracker
    FeedbackTracker  *SeedInterestTracker
    Logger           zerolog.Logger
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
2. Extract traffic → pipeline.Dict().ExtractFromTraffic(seeds)  (populates per-endpoint header dictionary)
3. Create IntensityTracker → pipeline.SetIntensityCallback(tracker.GetMultiplier)
4. Create SeedInterestTracker
5. Create anomaly MultiDetector
6. Create Triager
7. Create Replayer
8. Convert extraction rules
9. Create Limiter (token bucket)
10. Create task channel (WorkerCount * 2 capacity)
11. Spawn Workers → each runs w.Run(ctx, taskCh) with IntensityTracker + FeedbackTracker in config
12. Task generator loop:
   └─ for each iteration:
      ├── check ctx.Done()
      ├── check max_tests
      ├── check duration_sec deadline
      ├── limiter.Acquire(ctx) → blocks if over RPS
      ├── seed selection (epsilon-greedy):
      │   ├── 80%: weightedPick by FeedbackTracker.NormalizedWeights(seedIDs) (exploitation)
      │   └── 20%: uniform random (exploration)
      ├── generate random mutation seed
      └── taskCh ← SeedTask
13. close(taskCh) → workers detect closed channel → exit
14. wg.Wait() → all workers done
15. UpdateStatus → FINISHED (or STOPPED if ctx cancelled)
16. Vulnerability grouping (if store implements GroupingStore):
    └─ List all CONFIRMED findings → GroupFindings() → UpdateFindingGroup() per finding
17. Post-campaign LLM batch analysis (if llmTriager is not nil):
    └─ List all UNCONFIRMED findings → load artifact from disk → BatchAnalyze() → persist via UpdateLLMAnalysis()
    └─ Runs with a 10-minute timeout context
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
    │   ├── [adaptive] intensityTracker.RecordApplication(result.Operators)
    │   │
    │   ├── replayer.ReplayExchange(result.Exchange, baseURL, wctx) → ExchangeResult
    │   │
    │   ├── [feedback] feedbackTracker.RecordResponse(seedID, statusCode, errorBody)
    │   │
    │   ├── detector.Detect(exchange, result, baseline, anomalyCfg) → []AnomalyHit
    │   │
    │   └── For each hit → triage + persist:
    │       ├── [adaptive] intensityTracker.RecordFinding(ops)
    │       └── [feedback] feedbackTracker.RecordFinding(seedID)
    │
    └── campaigns.IncrementStats(campaignID, 1, findingsDelta)
```

## Adaptive Intensity & Feedback

### IntensityTracker

`IntensityTracker` (internal/engine/intensity.go) tracks per-operator productivity and computes dynamic multipliers for the mutation pipeline:

- **Operators tracked**: `uri`, `header`, `json`, `param`, `primitive`
- **Multiplier formula**: `1.0 + productivity*1.5 + explorationBonus`, capped at **2.5x**
  - `productivity = findings / max(1, applications)`
  - `explorationBonus = 0.5` if `< 10 applications`, otherwise `0`
- **Effect**: The pipeline multiplies its base `Intensity` by each operator's multiplier. Highly productive operators get more use; underexplored operators get a temporary boost.
- **Thread safety**: `sync.Mutex` protects shared state accessed by multiple workers.

### SeedInterestTracker

`SeedInterestTracker` (internal/engine/feedback.go) powers coverage-guided seed selection:

- **Scoring**: Seeds earn interest for novel responses:
  - Novel status code (not previously seen): **+2.0**
  - Novel error body signature: **+3.0**
  - Producing a finding: **+5.0**
- **Error signature**: SHA256 hash of first 512 bytes of error response body
- **Weighted selection**: `NormalizedWeights(seedIDs)` returns a probability distribution where high-interest seeds receive proportionally more probability mass

### Epsilon-Greedy Seed Selection

The task generator uses an epsilon-greedy strategy to balance exploration and exploitation:

- **80% exploitation**: `weightedPick(seeds, weights, rng)` — seeds with higher interest scores are more likely to be selected
- **20% exploration**: uniform random — ensures no seed is permanently ignored
- **`weightedPick[T any]`**: generic helper using cumulative distribution sampling

### Wiring

All wiring happens in `engine.go:runCampaign()`:
1. `pipeline.Dict().ExtractFromTraffic(seeds)` populates the dictionary from recorded traffic
2. `IntensityTracker` is created and wired via `pipeline.SetIntensityCallback(tracker.GetMultiplier)`
3. `SeedInterestTracker` is created and passed to each `Worker` via `WorkerConfig`
4. Workers call `RecordApplication`, `RecordResponse`, and `RecordFinding` during their fuzz loop

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
- `IntensityTracker` multipliers are capped at 2.5x. No operator ever receives more than a 2.5x boost regardless of productivity.
- `SeedInterestTracker` scores start at 1.0 (default weight). Seeds always have a non-zero probability of being selected, guaranteeing coverage of all seeds.
- When only one seed exists, epsilon-greedy degrades to uniform random (no weighted selection possible with a single seed).
- Vulnerability grouping is run at campaign stop via type assertion on `e.findings.(GroupingStore)`. If the store does not implement `GroupingStore` (e.g., mock stores in tests), grouping is silently skipped. A background goroutine in `cli/serve.go` also groups periodically every 15s.
- Post-campaign LLM batch analysis runs only when `llmTriager` is not nil (LLM is configured and enabled). It uses a separate 10-minute timeout context and lists only UNCONFIRMED findings. Artifacts are loaded from disk via `artifactGetter`. Each analysis result is persisted via `UpdateLLMAnalysis` using a fresh context (not the timeout context) so persistence is not affected by the LLM timeout.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/anomaly` | `Detector`, `MultiDetector`, `BaselineEntry`, `AnomalyHit` |
| `internal/corpus` | `Manager` for seed loading and baseline computation |
| `internal/model` | `Campaign`, `CampaignConfig`, `CampaignStatus`, `Finding`, `Artifact` |
| `internal/mutate` | `Pipeline`, `SeqMutator`, `Config` |
| `internal/replayer` | `Replayer`, `ExchangeResult`, `WorkerContext`, `ExtractionRule` |
| `internal/triage` | `Triager` for dedup, confirm, minimize, severity, categorization, grouping; `LLMTriager` for post-campaign LLM batch analysis |
| `internal/metrics` | `TestsTotal`, `FindingsTotal` |
| `encoding/json` | Artifact payload deserialization for LLM analysis |
| `os` | Artifact file read for LLM batch |

## Edge Cases

- **No seeds for campaign**: `StartCampaign` returns error "no seeds", campaign transitions to FAILED.
- **RPS = 0**: Limiter's `Acquire()` returns immediately — no rate limiting.
- **Worker count not set**: Defaults to 4 workers.
- **MaxTests and DurationSec both 0**: Campaign runs indefinitely until manually stopped.
- **Reproduce worker encounters corrupt artifact**: Logs error and marks reproduce status as FAILED. Continues polling.
- **Duplicate findings**: `ExistsBySignature` check prevents duplicate `Create` calls. Two workers detecting the same anomaly on different exchanges produce only one finding.
