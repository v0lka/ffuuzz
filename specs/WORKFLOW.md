# WORKFLOW

This document describes step-by-step workflows for common development tasks in FFUUZZ. Follow the relevant workflow when making changes. Each workflow references the specs you should read before starting and the files you will likely modify.

Before starting any workflow, read [`META.md`](META.md) to understand spec conventions and [`architecture/layers.md`](architecture/layers.md) to understand the layer architecture.

---

## 1. Adding a New Anomaly Detector

### Read before starting
- [`domains/anomaly-detection.md`](domains/anomaly-detection.md) — detector interface and existing detectors

### Steps

1. **Define the detector type** in `internal/anomaly/detector.go`:
   - Create a new struct (e.g., `StatusCodeDetector`)
   - Implement the `Detector` interface: `Detect(ex, result, baseline, cfg) []AnomalyHit`
   - Add any new finding types to `internal/model/model.go` if needed

2. **Register in MultiDetector**: Update `NewMultiDetector` to include the new detector, gated by the appropriate `AnomalyConfig` field.

3. **Add config field** in `model.AnomalyConfig` (e.g., `DetectRedirects bool`).

4. **Update the API**: Add the new config field to campaign creation/update validation in `internal/api/campaigns.go`.

5. **Update the frontend**: Add UI toggle for the new detector in the campaign create form (`web/src/pages/CampaignCreatePage.tsx`).

6. **Write tests**: Add test cases in `internal/anomaly/detector_test.go` following the existing table-driven pattern.

7. **Run checks**: `make lint && make test`

---

## 2. Adding a New Mutation Strategy

### Read before starting
- [`domains/fuzzing-engine/mutate.md`](domains/fuzzing-engine/mutate.md) — mutation pipeline and existing strategies
- [`domains/fuzzing-engine/engine.md`](domains/fuzzing-engine/engine.md) — how the engine uses the pipeline

### Steps

1. **Create the mutator** in `internal/mutate/`:
   - Create a new file (e.g., `cookie_mutate.go`)
   - Implement `ExchangeMutator` interface
   - Return `MutationResult` with the mutated exchange and operator names

2. **Add config field** in `mutate.Config` and `model.MutationConfig` (e.g., `Cookies bool`).

3. **Register in Pipeline**: Update `NewPipeline` to create the mutator and `Pipeline.Mutate` to call it (gated by the config field and intensity).

4. **Update the API and frontend** — same pattern as Adding a Detector.

5. **Write a fuzz test** that verifies the mutator produces different output from input.

6. **Run checks**: `make lint && make test`

---

## 3. Adding a New API Endpoint

### Read before starting
- [`contracts/api-db.md`](contracts/api-db.md) — API → DB store interfaces
- [`contracts/api-engine.md`](contracts/api-engine.md) — API → Engine boundary

### Steps

1. **Define the handler** in the appropriate `internal/api/` file (e.g., `internal/api/recordings.go`):
   - Write a method on `*Server`
   - Use `s.parsePagination(c)` for list endpoints
   - Return errors via `errorResponse(c, status, code, msg)` or `s.internalError(c, code, err)`

2. **Register the route** in `internal/api/server.go` `NewServer()`, under the appropriate `v1` group.

3. **If it needs a new store method**:
   - Add the method to the interface in `internal/api/server.go` (e.g., `RecordingStore`)
   - Implement it in `internal/db/` (e.g., `internal/db/recordings.go`)
   - Write a DB test using `sqlmock`

4. **Update the frontend**:
   - Add the API call function in `web/src/api/`
   - Create or update the page component in `web/src/pages/`
   - Add a route in `web/src/router.tsx` if it's a new page

5. **Write API handler tests** in `internal/api/`.

6. **Run checks**: `make lint && make test`

---

## 4. Changing Campaign Lifecycle

### Read before starting
- [`domains/campaign-management.md`](domains/campaign-management.md) — status lifecycle and API
- [`domains/fuzzing-engine/engine.md`](domains/fuzzing-engine/engine.md) — engine implementation
- [`contracts/engine-stores.md`](contracts/engine-stores.md) — engine store interfaces

### Steps

1. **Update the status constants** if adding a new status in `internal/model/model.go` (e.g., `CampaignPaused`).

2. **Add status transition logic** in `internal/engine/engine.go`:
   - New methods for transitions (e.g., `PauseCampaign`, `ResumeCampaign`)
   - Use `campaignStore.UpdateStatus(ctx, id, oldStatus, newStatus)` — always optimistic locking

3. **Update graceful shutdown** in `internal/engine/engine.go` `StopAll()` to handle the new status.

4. **Update the API**:
   - New endpoint handler(s) in `internal/api/campaigns.go`
   - Register routes in `internal/api/server.go`

5. **Update the frontend**: Status display, transition buttons, SSE event handling.

6. **Update DB** if new columns are needed (add a migration in `internal/db/migrations/`).

7. **Run checks**: `make lint && make test`

---

## 5. Modifying the MITM Proxy

### Read before starting
- [`domains/traffic-capture/mitm.md`](domains/traffic-capture/mitm.md) — proxy implementation
- [`domains/traffic-capture/recorder.md`](domains/traffic-capture/recorder.md) — recorder interface
- [`architecture/security-model.md`](architecture/security-model.md) — TLS interception details
- [`contracts/proxy-recorder.md`](contracts/proxy-recorder.md) — proxy → recorder contract

### Steps

1. **Modify proxy logic** in `internal/mitm/mitm.go`:
   - HTTP handling: `handleHTTP()`
   - HTTPS handling: `handleCONNECT()`

2. **If changing `TxRecord` fields**:
   - Update `TxRecord` struct in `internal/recorder/recorder.go`
   - Update `TxRecordToExchange()` and `ExchangeToTxRecord()` conversion functions
   - Update `DBRecorder.Record()` to populate new fields

3. **If changing the `Recorder` interface**:
   - Update both implementations (`DBRecorder`, file recorder)
   - Update MITM proxy callsites

4. **Add metrics** in `internal/metrics/` if the change introduces new observability needs.

5. **Write integration-level tests** in `internal/mitm/mitm_test.go` (use `httptest.Server` as upstream).

6. **Run checks**: `make lint && make test`

---

## 6. Modifying Database Schema

### Read before starting
- [`contracts/api-db.md`](contracts/api-db.md) — API store interfaces
- [`contracts/engine-stores.md`](contracts/engine-stores.md) — engine store interfaces
- [`decisions/002-postgresql-persistence.md`](decisions/002-postgresql-persistence.md) — PostgreSQL decision context

### Steps

1. **Add a migration** in `internal/db/migrations/` with an incremented version number (e.g., `000002_add_column.up.sql` and `.down.sql`).

2. **Update Go models** in `internal/model/model.go` to include new fields with appropriate `json:"..." db:"..."` tags.

3. **Update store implementations** in `internal/db/`:
   - Modify SQL queries to include new columns
   - Update `sqlx.StructScan` destinations

4. **Update interfaces** in `internal/api/server.go` or `internal/engine/engine.go` if the new columns affect method signatures.

5. **Update tests**: All `sqlmock`-based tests must include the new columns in their expected queries.

6. **Test migration**: Run `./ffuuzz serve` with a fresh database to verify migrations apply cleanly.

7. **Run checks**: `make lint && make test`

---

## 7. Adding a New Package

### Read before starting
- [`architecture/layers.md`](architecture/layers.md) — layer definitions and import rules
- [`META.md`](META.md) — spec creation guidelines

### Steps

1. **Determine the layer**: Is it Boundary, Domain, Infrastructure, or Utility? See [`architecture/layers.md`](architecture/layers.md) for criteria.

2. **Create the package** under `internal/` with appropriate imports:
   - Domain: may import `model`, utility packages, other domain packages
   - Infrastructure: may import `model`, utility, domain packages
   - Utility: zero internal imports
   - Boundary: may import anything

3. **Wire it in** `internal/cli/serve.go` if it needs initialization.

4. **Create the spec**:
   - If it's a new domain component: create a Domain Detail file (see [`META.md`](META.md) template)
   - If it's infrastructure: update relevant contract specs
   - If it adds a new boundary: update [`contracts/cli-infrastructure.md`](contracts/cli-infrastructure.md)

5. **Update INDEX.md** with the new spec file.

6. **Run checks**: `make lint && make test`

---

## 8. General Code Change Workflow

For changes that don't fit the workflows above:

1. **Read the relevant specs** from [`INDEX.md`](INDEX.md).
2. **Understand the layer** your change affects — check [`architecture/layers.md`](architecture/layers.md).
3. **Check contract specs** for any interface you're modifying.
4. **Make the change** following the patterns in existing code.
5. **Run `make lint && make test`**.
6. **Update specs** if the change affects documented behaviour, invariants, or interfaces.
