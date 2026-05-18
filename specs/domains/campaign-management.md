# Campaign Management

## Overview

Campaigns are the central orchestration entity in FFUUZZ. A campaign defines a fuzzing run: which target to test, which recording sessions to use as seeds, which mutation strategies to apply, which anomaly detectors to activate, and how to triage findings. The campaign lifecycle is managed through the Control API and driven by the `engine.Engine`.

## Key Files

| File | Role |
|------|------|
| `internal/model/model.go` | `Campaign`, `CampaignConfig`, `CampaignStatus`, `CampaignStats`, `CampaignLimits`, `CampaignProgress` |
| `internal/engine/engine.go` | `Engine.StartCampaign`, `Engine.StopCampaign`, `Engine.StopAll`, `runCampaign()` |
| `internal/api/campaigns.go` | API handlers for campaign CRUD, start/stop, stats, stream |
| `internal/db/campaigns.go` | `CampaignStore` PostgreSQL implementation |

## Core Types

```go
type CampaignStatus string
const (
    CampaignCreated  CampaignStatus = "CREATED"
    CampaignStarting CampaignStatus = "STARTING"
    CampaignRunning  CampaignStatus = "RUNNING"
    CampaignStopping CampaignStatus = "STOPPING"
    CampaignStopped  CampaignStatus = "STOPPED"
    CampaignFinished CampaignStatus = "FINISHED"
    CampaignFailed   CampaignStatus = "FAILED"
)

type Campaign struct {
    ID           string
    Name         string
    Status       CampaignStatus
    CreatedAt    time.Time
    UpdatedAt    time.Time
    StartedAt    *time.Time
    FinishedAt   *time.Time
    RecordingIDs []string
    Config       CampaignConfig
    Progress     *CampaignProgress
}

type CampaignConfig struct {
    Target          TargetURL
    Limits          CampaignLimits
    Mutations       MutationConfig
    Anomaly         AnomalyConfig
    Triage          TriageConfig
    ExtractionRules []ExtractionRule
}

type CampaignStats struct {
    CampaignID         string
    Status             CampaignStatus
    TestsTotal         int
    TestsPerSec        float64
    Timeouts           int
    ServerErrors       int
    LatencyRegressions int
    RegexMatches       int
    LastActivityAt     time.Time
    Seeds              SeedStats
}
```

## Status Lifecycle

```
CREATED ──► STARTING ──► RUNNING ──► FINISHED  (normal completion)
                  │          │
                  │          └──► STOPPING ──► STOPPED  (user-initiated stop)
                  │
                  └──► FAILED  (seed load or transition error)
```

Status transitions use optimistic locking: `UpdateStatus(ctx, id, expectedOldStatus, newStatus)` returns `(ok, error)`. The where clause in the SQL compares `status = $oldStatus`. If another process changed the status concurrently, the update has zero rows affected and `ok = false`.

## API Endpoints

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/api/v1/campaigns` | `createCampaign` | Create a new campaign in CREATED state |
| GET | `/api/v1/campaigns` | `listCampaigns` | List campaigns with pagination and status filter |
| GET | `/api/v1/campaigns/:id` | `getCampaign` | Get a single campaign by ID |
| GET | `/api/v1/campaigns/:id/stats` | `getCampaignStats` | Real-time stats: tests, findings by type, rate |
| GET | `/api/v1/campaigns/:id/findings` | `getCampaignFindings` | List findings for a campaign |
| GET | `/api/v1/campaigns/:id/config` | `getCampaignConfig` | Get the campaign's full config |
| GET | `/api/v1/campaigns/:id/stream` | `streamCampaignStats` | SSE stream of stats updates |
| POST | `/api/v1/campaigns/:id/start` | `startCampaign` | Start fuzzing (CREATED → STARTING → RUNNING) |
| POST | `/api/v1/campaigns/:id/stop` | `stopCampaign` | Stop fuzzing (RUNNING → STOPPING → STOPPED) |
| POST | `/api/v1/campaigns/:id/recordings` | `addRecordingsToCampaign` | Add recording sessions by origin/path filter |

## SSE Streaming

The `/api/v1/campaigns/:id/stream` endpoint streams campaign stats to the frontend dashboard:

1. Client opens SSE connection (GET with `Accept: text/event-stream`)
2. Server sends `event: stats` with `CampaignStats` JSON payload every 2 seconds
3. Server sends `event: done` when the campaign is no longer running
4. Server closes the connection

## Invariants

- Campaign creation does NOT start fuzzing. The campaign is created in `CREATED` state and must be explicitly started via `POST .../start`.
- `StartCampaign` is synchronous for status transitions and seed loading, then asynchronous for the fuzz loop (`go runCampaign()`). The API caller gets a response after the campaign reaches `RUNNING` state.
- Status transitions use optimistic locking. Concurrent `start` and `stop` requests cannot race each other.
- `StopCampaign` cancels the campaign context, which propagates to all workers. Workers drain their current mutation cycle before exiting.
- A campaign can only be stopped when it is `RUNNING` (or `STARTING` as a fallback). A `FINISHED` or `STOPPED` campaign cannot be stopped again.
- `StopAll` is called during shutdown and transitions all running campaigns to `STOPPING`.
- `CampaignConfig` is stored as JSONB in the database. Go struct tags use `json:"-"` for the struct field and `db:"config"` for the JSONB column to avoid double serialization.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/model` | `Campaign`, `CampaignConfig`, `CampaignStatus`, etc. |
| `internal/engine` | Campaign lifecycle orchestration |
| `internal/api` | REST API handlers |
| `internal/db` | PostgreSQL persistence |
| `internal/metrics` | `TestsTotal`, `FindingsTotal` counters |

## Edge Cases

- **Start with no recordings**: Campaign transitions to `FAILED`.
- **Start an already-running campaign**: Returns error (status is not `CREATED`).
- **Stop a non-running campaign**: Returns error (status is not `RUNNING` or `STARTING`).
- **Campaign finishes (max_tests or duration_sec)**: Transitions to `FINISHED`. SSE sends `done` event.
- **Process dies during campaign**: Campaign remains in `RUNNING` in DB. On restart, no reconciliation — the campaign must be stopped and re-created.
