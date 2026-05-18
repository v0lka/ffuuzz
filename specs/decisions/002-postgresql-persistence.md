# ADR-002: PostgreSQL Persistence

## Status
Accepted

## Context
FFUUZZ needs to persist recorded HTTP sessions, campaign configurations, fuzzing findings, and artifacts. The data model includes JSONB fields (campaign config, finding details, request/response headers) and requires atomic status transitions for campaign lifecycle and reproduce job claiming. The tool targets local/CI environments where a single database instance is sufficient.

## Decision
Use PostgreSQL 16 as the sole persistence layer. Access PostgreSQL through `jmoiron/sqlx` for ergonomic SQL with struct scanning. Handle migrations with `golang-migrate/migrate` using embedded SQL files. Store complex nested structures (campaign config, finding details, headers) as JSONB columns.

**Store pattern**: Each entity type (recordings, campaigns, findings, artifacts) has its own store struct in `internal/db` with dedicated methods. No ORM — all queries are hand-written SQL.

**Optimistic locking**: Campaign status transitions use `UPDATE ... WHERE status = $oldStatus` to prevent concurrent modification races. Reproduce job claiming uses `SELECT ... FOR UPDATE SKIP LOCKED`.

## Consequences

### Positive
- JSONB columns allow flexible schema for config and details without JOIN complexity
- `sqlx.StructScan` reduces boilerplate while keeping SQL explicit and reviewable
- Embedded migrations (`//go:embed migrations/*.sql`) ensure the binary is self-contained
- `FOR UPDATE SKIP LOCKED` enables safe concurrent reproduce job consumption
- Optimistic locking prevents campaign status race conditions without row-level locks

### Negative
- PostgreSQL is a hard runtime dependency — `docker-compose.yml` provides one, but the tool won't start without it
- Hand-written SQL is more verbose than an ORM for CRUD operations
- No built-in migration rollback strategy in the embedded migration system
- `sqlmock` is required for unit-testing database code, adding mock setup complexity
- JSONB queries (e.g., filtering by campaign status in config) require PostgreSQL-specific syntax

## Alternatives Considered

- **SQLite**: Simpler deployment (no external process). Rejected because JSONB operations are less mature, and `FOR UPDATE SKIP LOCKED` semantics differ.
- **GORM or other ORM**: Would reduce boilerplate but obscure query behavior. Rejected in favor of explicit SQL for security tooling where data integrity is critical.
- **MongoDB**: Native JSON storage would simplify the JSONB pattern. Rejected to keep operational dependencies minimal (PostgreSQL is already required).
- **File-based storage (BoltDB, Badger)**: No external process dependency. Rejected because the web dashboard requires query flexibility (filtering, sorting, aggregation).
