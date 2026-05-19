# Engine → Stores

## Overview

The engine depends on three store interfaces for database operations during campaign execution. Each interface is defined in `internal/engine` and implemented by `internal/db`. The engine owns narrower interfaces than the API — it only needs the methods relevant to campaign orchestration and finding persistence.

## Interfaces

All defined in `internal/engine/engine.go`:

```go
// CampaignStore — campaign lifecycle operations
type CampaignStore interface {
    UpdateStatus(ctx context.Context, id string,
        oldStatus, newStatus model.CampaignStatus) (bool, error)
    IncrementStats(ctx context.Context, id string,
        testsDelta, findingsDelta int) error
}

// FindingStore — finding CRUD and reproduce jobs
type FindingStore interface {
    ExistsBySignature(ctx context.Context, campaignID, signature string) (bool, error)
    Create(ctx context.Context, f model.Finding) error
    UpdateStatus(ctx context.Context, id string, status model.FindingStatus) error
    GetByID(ctx context.Context, id string) (*model.Finding, error)
    ClaimNextReproduceJob(ctx context.Context) (string, int, bool, error)
    SetReproduceStatus(ctx context.Context, id, status string) error
}

// GroupingStore — extends FindingStore with grouping operations for vulnerability grouping.
// Used via type assertion; not required for all FindingStore implementations.
type GroupingStore interface {
    FindingStore
    ListAll(ctx context.Context, campaignID, typeFilter, statusFilter string,
        since *time.Time, limit, offset int) ([]model.Finding, error)
    UpdateFindingGroup(ctx context.Context, id, groupID string) error
}

// ArtifactStore — artifact persistence
type ArtifactStore interface {
    Create(ctx context.Context, a model.Artifact) error
    GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error)
}
```

## Comparison with API Interfaces

| Operation | Engine Interface | API Interface |
|-----------|-----------------|---------------|
| Get campaign | — (not needed) | `GetByID` |
| Create campaign | — | `Create` |
| List campaigns | — | `List` |
| Update status | `UpdateStatus` (optimistic lock) | — |
| Increment stats | `IncrementStats` (atomic) | — |
| Dedup findings | `ExistsBySignature` | — |
| Create finding | `Create` | — |
| Update finding status | `UpdateStatus` | — |
| List findings | `ListAll` (GroupingStore) | `ListAll` |
| Get finding | `GetByID` | `GetByID` |
| Reproduce jobs | `ClaimNextReproduceJob`, `SetReproduceStatus` | `UpdateReproduceStatus` |
| Count by type | — | `CountByType` |
| Group findings | `UpdateFindingGroup` (GroupingStore) | `UpdateFindingGroup` |

The engine has a narrower interface focused on write-heavy campaign execution, but the `GroupingStore` extension adds `ListAll` for vulnerability grouping at campaign stop.

## Implementations

| Interface | Implementation | Package |
|-----------|---------------|---------|
| `engine.CampaignStore` | `db.CampaignStore` | `internal/db` |
| `engine.FindingStore` | `db.FindingStore` | `internal/db` |
| `engine.GroupingStore` | `db.FindingStore` | `internal/db` |
| `engine.ArtifactStore` | `db.ArtifactStore` | `internal/db` |

## Initialization

In `cli/serve.go`, the same `db.*Store` instances are passed to both:

```go
campaignStore := db.NewCampaignStore(database.DB, logger)  // implements both api.CampaignStore and engine.CampaignStore
findingStore  := db.NewFindingStore(database.DB, logger)    // implements both api.FindingStore and engine.FindingStore
artifactStore := db.NewArtifactStore(database.DB, logger)   // implements both api.ArtifactStore and engine.ArtifactStore

eng := engine.NewEngine(campaignStore, findingStore, artifactStore, ...)
```

## Data Flow

```
Worker (fuzz loop):
    │
    ├── findings.ExistsBySignature(ctx, campaignID, signature)
    │   └── SQL: SELECT EXISTS(SELECT 1 FROM findings WHERE campaign_id=$1 AND signature=$2)
    │   └── Returns bool (true = skip, already exists)
    │
    ├── findings.Create(ctx, finding)
    │   └── SQL: INSERT INTO findings (id, campaign_id, type, signature, ...)
    │   └── Returns error
    │
    ├── artifacts.Create(ctx, artifact)
    │   └── SQL: INSERT INTO artifacts (id, finding_id, file_path, size_bytes)
    │   └── Returns error
    │
    ├── campaigns.IncrementStats(ctx, campaignID, 1, findingsDelta)
    │   └── SQL: UPDATE campaigns SET tests_done = tests_done + $1, findings_total = findings_total + $2
    │   └── Returns error
    │
    └── findings.UpdateStatus(ctx, findingID, model.FindingConfirmed)
        └── SQL: UPDATE findings SET status=$1 WHERE id=$2
        └── Returns error

ReproduceWorker:
    │
    ├── findings.ClaimNextReproduceJob(ctx)
    │   └── Atomic: SELECT id, reproduce_runs FROM findings
    │               WHERE reproduce_status='ENQUEUED' ORDER BY reproduce_enqueued_at
    │               FOR UPDATE SKIP LOCKED LIMIT 1
    │               UPDATE reproduce_status='RUNNING'
    │   └── Returns (findingID, runs, hasJob, error)
    │
    └── findings.SetReproduceStatus(ctx, id, status)
        └── SQL: UPDATE findings SET reproduce_status=$1

Campaign stop (grouping):
    │
    ├── findings.ListAll(ctx, campaignID, "", "CONFIRMED", nil, 10000, 0)
    │   └── SQL: SELECT * FROM findings WHERE campaign_id=$1 AND status=$2
    │   └── Returns []Finding (all confirmed findings)
    │
    └── findings.UpdateFindingGroup(ctx, findingID, groupID)
        └── SQL: UPDATE findings SET group_id=$1 WHERE id=$2
        └── Returns error

Periodic grouping (15s loop):
    │
    ├── findings.ListAll(ctx, "", "", "CONFIRMED", nil, 10000, 0)
    │   └── SQL: SELECT * FROM findings WHERE status='CONFIRMED'
    │   └── Returns []Finding (filtered for group_id IS NULL)
    │
    └── findings.UpdateFindingGroup(ctx, findingID, groupID)
        └── SQL: UPDATE findings SET group_id=$1 WHERE id=$2
        └── Returns error
```

## Invariants

- `UpdateStatus` uses optimistic locking (`WHERE status = $oldStatus`). If the campaign status was changed by another process, `ok = false` is returned and no update occurs.
- `ExistsBySignature` is called before `Create`. The engine relies on this for dedup, not on DB unique constraints (though the DB likely also has a unique index on signature).
- `ClaimNextReproduceJob` is atomic with `FOR UPDATE SKIP LOCKED`. Multiple reproduce workers (if ever added) cannot claim the same job.
- `IncrementStats` is a single atomic UPDATE. No read-modify-write race.
- `GroupingStore` is consumed via type assertion (`e.findings.(GroupingStore)`). If the store does not implement it, grouping is silently skipped. Mock stores in tests typically do not implement `GroupingStore`.

## Breaking Change Checklist

- [ ] Does the new interface method exist on `db.CampaignStore`?
- [ ] Does the new interface method exist on `db.FindingStore`?
- [ ] Does the new interface method exist on `db.ArtifactStore`?
- [ ] Are optimistic locking semantics preserved for status transitions?
- [ ] Is `ClaimNextReproduceJob` still atomic with `SKIP LOCKED`?
- [ ] Are all write operations safe for concurrent workers? (they should be — PostgreSQL handles row-level locking)

## Related

- [`api-db.md`](api-db.md) — API → DB boundary (overlapping but distinct interfaces)
- [`api-engine.md`](api-engine.md) — API → Engine boundary
- [`domains/fuzzing-engine/engine.md`](../domains/fuzzing-engine/engine.md) — engine implementation
