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
| POST | `/api/v1/campaigns/quick` | `quickCreateCampaign` | Create a campaign from an origin/path filter with default config |
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
| POST | `/api/v1/campaigns/:id` | `editCampaign` | Update campaign name, recording IDs, or config |
| POST | `/api/v1/campaigns/:id/analyze` | `analyzeCampaign` | Batch LLM-analyze all unconfirmed findings |

### Create Campaign

The `POST /api/v1/campaigns/quick` endpoint creates a campaign from a filter instead of requiring explicit recording IDs:

**Request:**
```json
{
  "name": "campaign name",
  "filter": {
    "scheme": "https",
    "host": "example.com",
    "port": 443,
    "path_prefix": "/api"
  }
}
```

**Behavior:**
- The handler builds a `Campaign` with sensible defaults (workers=8, rps=50, max_tests=10000, etc.)
- Target `base_url` is auto-derived from `scheme://host:port`
- Calls `CampaignStore.CreateWithFilter()` which executes in a single transaction: inserts the campaign row, then `INSERT INTO campaign_recordings SELECT FROM recordings WHERE filter`
- Returns 201 with the created campaign on success
- Returns 400 if no recordings match the filter (transaction rolls back)

**Default config applied:**
| Setting | Value |
|---------|-------|
| workers | 8 |
| rps | 50 |
| max_tests | 10000 |
| req_timeout_ms | 3000 |
| mutations.path_query | true |
| mutations.headers | true |
| mutations.json_body | true |
| mutations.params | true |
| mutations.intensity | 0.6 |
| anomaly.detect_5xx | true |
| anomaly.latency_multiplier | 3.0 |
| triage.confirm_runs | 3 |
| triage.enable_minimization | true |

### Edit Campaign

The `POST /api/v1/campaigns/:id` endpoint updates an existing campaign. All fields are optional; at least one of `name`, `recording_ids`, or `config` must be provided.

**Request:**
```json
{
  "name": "Updated Name",
  "recording_ids": ["uuid-1", "uuid-2"],
  "config": { "target": {"base_url": "http://new-target:8080"}, "limits": {"workers": 4, "rps": 20, "max_tests": 5000, "duration_sec": 0, "req_timeout_ms": 5000}, "mutations": {"path_query": true, "headers": false, "json_body": true, "params": true, "sequence": false, "intensity": 0.8}, "anomaly": {"detect_5xx": true, "latency_multiplier": 2.0, "regex_patterns": ["error"]}, "triage": {"confirm_runs": 5, "enable_minimization": true} }
}
```

**Behavior:**
- Fetches the existing campaign via `GetByID`. Returns 404 if not found.
- Editing is allowed only when the campaign status is `CREATED`, `STOPPED`, `FINISHED`, or `FAILED`. Active campaigns (`RUNNING`, `STARTING`, `STOPPING`) return 409 `INVALID_STATE`.
- If `name` is provided, it is validated (max 255 characters) and applied.
- If `recording_ids` is provided, each ID is validated against the recordings store (returns 404 for unknown IDs) and replaces the existing recording links.
- If `config` is provided, it is validated via `validateCampaignConfig` and replaces the existing configuration.
- The `updated_at` timestamp is set to the current time.
- Recording links are replaced atomically in a transaction: old links are deleted, new links are inserted.
- Returns 200 with the updated campaign on success.

**Validation rules:**
- At least one field required (400 `INVALID_BODY`)
- Name max 255 characters (400 `NAME_TOO_LONG`)
- All recording IDs must exist (404 `RECORDING_NOT_FOUND`)
- Config must satisfy all create-time constraints (422 `INVALID_CONFIG`)
- Campaign must be in an editable state (409 `INVALID_STATE`)

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
- Campaign editing is allowed only for non-active campaigns (`CREATED`, `STOPPED`, `FINISHED`, `FAILED`). Active campaigns (`RUNNING`, `STARTING`, `STOPPING`) reject edits with 409.
- Editing preserves progress counters (`tests_done`, `findings_total`) — only `name`, `config`, and `recording_ids` are updated.
- Recording link replacement is atomic (delete all old, insert all new in a single transaction).

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
- **Edit an active campaign**: Returns 409 `INVALID_STATE`. Edit button is hidden in the UI for `RUNNING`, `STARTING`, and `STOPPING` campaigns.
- **Edit with empty body**: Returns 400 `INVALID_BODY` — at least one field must be provided.
- **Edit only name**: Only `name` is updated; `config` and `recording_ids` remain unchanged.
- **Edit only recordings**: Recording links are replaced atomically; `name` and `config` remain unchanged.
- **Edit after campaign completion**: Allowed — a `FINISHED` campaign can be reconfigured and re-started.
- **Remove all recordings via edit**: Allowed — `recording_ids` can be set to an empty array. The campaign must have recordings to start.
