# API → DB

## Overview

The API server depends on four store interfaces for database operations. Each interface is defined in `internal/api` and implemented by `internal/db`. This follows the Dependency Inversion Principle: the API (consumer) owns the interface contract; the DB package (provider) implements it.

## Interfaces

All defined in `internal/api/server.go`:

```go
// RecordingStore — recordings CRUD
type RecordingStore interface {
    GetByID(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error)
    GetByIDs(ctx context.Context, ids []string) ([]model.RecordingSession, error)
    Upsert(ctx context.Context, sess model.RecordingSession) (bool, error)
    List(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error)
    ListAll(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error)
    Delete(ctx context.Context, id string) (bool, error)
    IsUsedByActiveCampaign(ctx context.Context, id string) (bool, error)
    GetTree(ctx context.Context) ([]model.TreeEntry, error)
    DeleteByPrefix(ctx context.Context, scheme, host string, port int, pathPrefix string) (int64, error)
}

// CampaignStore — campaign CRUD
type CampaignStore interface {
    GetByID(ctx context.Context, id string) (*model.Campaign, error)
    Create(ctx context.Context, c model.Campaign) error
    CreateWithFilter(ctx context.Context, c model.Campaign, scheme, host string, port int, pathPrefix string) (int, error)
    List(ctx context.Context, statusFilter string, limit, offset int) ([]model.Campaign, error)
    AddRecordingsByFilter(ctx context.Context, campaignID, scheme, host string, port int, pathPrefix string) (int, error)
    Update(ctx context.Context, c model.Campaign) error
}

// FindingStore — finding queries
type FindingStore interface {
    ListAll(ctx context.Context, campaignID, typeFilter, statusFilter string,
        since *time.Time, limit, offset int) ([]model.Finding, error)
    GetByID(ctx context.Context, id string) (*model.Finding, error)
    UpdateReproduceStatus(ctx context.Context, id, status string, runs int) error
    CountByType(ctx context.Context, campaignID string) (map[model.FindingType]int, error)
    UpdateFindingGroup(ctx context.Context, id, groupID string) error
    UpdateLLMAnalysis(ctx context.Context, id string, analysisJSON []byte) error
}

// ArtifactStore — artifact queries
type ArtifactStore interface {
    GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error)
}

// HealthChecker — database health
type HealthChecker interface {
    Ping(ctx context.Context) error
}
```

## Implementations

| Interface | Implementation | Package | Notes |
|-----------|---------------|---------|-------|
| `RecordingStore` | `db.RecordingStore` | `internal/db` | PostgreSQL, sqlx |
| `CampaignStore` | `db.CampaignStore` | `internal/db` | PostgreSQL, optimistic locking |
| `FindingStore` | `db.FindingStore` | `internal/db` | PostgreSQL |
| `ArtifactStore` | `db.ArtifactStore` | `internal/db` | PostgreSQL |
| `HealthChecker` | `db.Database` | `internal/db` | `sqlx.DB.Ping` |

## Initialization

In `cli/serve.go`:

```go
recordingStore := db.NewRecordingStore(database.DB, logger)
campaignStore  := db.NewCampaignStore(database.DB, logger)
findingStore   := db.NewFindingStore(database.DB, logger)
artifactStore  := db.NewArtifactStore(database.DB, logger)

apiSrv := api.NewServer(api.ServerConfig{
    Recordings:  recordingStore,   // RecordingStore
    Campaigns:   campaignStore,    // CampaignStore
    Findings:    findingStore,     // FindingStore
    Artifacts:   artifactStore,    // ArtifactStore
    Health:      database,         // HealthChecker
    // ...
})
```

## Data Flow

```
API Handler
    │
    ├── recordingStore.GetByID(ctx, id, includeEntries, maxBodyBytes)
    │   └── SQL: SELECT * FROM recording_sessions + exchanges JOIN
    │   └── Returns *RecordingSession
    │
    ├── campaignStore.Create(ctx, campaign)
    │   └── SQL: INSERT INTO campaigns (id, name, status, config, ...)
    │   └── Returns error
    │
    ├── campaignStore.CreateWithFilter(ctx, campaign, scheme, host, port, pathPrefix)
    │   └── SQL: INSERT INTO campaigns ... + INSERT INTO campaign_recordings SELECT FROM recordings WHERE filter (in tx)
    │   └── Returns count of linked recordings (error if zero, transaction rolls back)
    │
    ├── campaignStore.Update(ctx, campaign)
    │   └── SQL: UPDATE campaigns SET name, updated_at, config WHERE id (in tx)
    │   └── SQL: DELETE FROM campaign_recordings WHERE campaign_id + INSERT for each recording_id
    │   └── Returns error
    │
    ├── findingStore.ListAll(ctx, campaignID, typeFilter, statusFilter, since, limit, offset)
    │   └── SQL: SELECT * FROM findings WHERE ... ORDER BY created_at DESC LIMIT $n OFFSET $n
    │   └── Returns []Finding
    │
    ├── findingStore.UpdateFindingGroup(ctx, id, groupID)
    │   └── SQL: UPDATE findings SET group_id=$1 WHERE id=$2
    │   └── Returns error
    │
    ├── findingStore.UpdateLLMAnalysis(ctx, id, analysisJSON)
    │   └── SQL: UPDATE findings SET llm_analysis=$1 WHERE id=$2
    │   └── Returns error
    │
    └── health.Ping(ctx)
        └── SQL: SELECT 1
        └── Returns error (nil = healthy)
```

## Pagination

API applies pagination to list endpoints:
- `limit`: default 50, max 50 (prevents OOM)
- `offset`: default 0, max 1,000,000
- Empty results: HTTP 204 No Content

The `List` and `ListAll` methods on the stores accept raw limit/offset values. The API handles validation and capping.

## Breaking Change Checklist

- [ ] Is the new interface method needed by the API?
- [ ] Is the new method implemented in `db`?
- [ ] Does the method signature match between API interface and DB implementation?
- [ ] Are error semantics consistent? (DB errors are returned as-is; API wraps them with `internalError()`)
- [ ] Does the new method have appropriate context cancellation support?
- [ ] Are all methods idempotent where they should be?

## Related

- [`api-engine.md`](api-engine.md) — API → Engine boundary
- [`cli-infrastructure.md`](cli-infrastructure.md) — wiring at startup
- [`engine-stores.md`](engine-stores.md) — Engine → DB stores (separate interfaces)
