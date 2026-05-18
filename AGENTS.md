# FFUUZZ Development Guide

## Specifications

Detailed system specs live in `specs/`. Before making structural changes, read the relevant spec:

- Start with [`specs/INDEX.md`](specs/INDEX.md) to find the right document for your task.
- [`specs/META.md`](specs/META.md) defines spec formats, layer architecture, and update rules.
- [`specs/WORKFLOW.md`](specs/WORKFLOW.md) provides step-by-step workflows for common development tasks.

## Quick Reference

### Building and Running

```bash
make build                  # Build frontend + backend → ./ffuuzz
make test                   # Run all Go tests with race detector
make lint                   # Lint frontend and backend
docker-compose up -d        # Start PostgreSQL
./ffuuzz serve              # Run the full application
```

### Project Structure

```
ffuuzz/
├── cmd/ffuuzz/main.go      # Entry point
├── internal/               # All Go packages (see specs/architecture/layers.md)
│   ├── cli/                # CLI entry point, composition root
│   ├── api/                # Gin REST API + SPA serving
│   ├── engine/             # Fuzzing engine + worker pool
│   ├── mitm/               # MITM proxy
│   ├── mutate/             # Mutation pipeline
│   ├── anomaly/            # Anomaly detectors
│   ├── triage/             # Finding triage + minimization
│   ├── model/              # Core domain types (zero internal deps)
│   ├── db/                 # PostgreSQL stores + migrations
│   ├── recorder/           # Traffic recording
│   ├── replayer/           # HTTP replay
│   ├── corpus/             # Seed loading + baselines
│   ├── endpoint/           # Path normalization + trie
│   ├── store/              # TLS cert store
│   ├── config/             # Configuration loading
│   ├── metrics/            # Prometheus metrics
│   ├── logging/            # Zerolog factory
│   ├── httputil/           # HTTP helpers
│   ├── diff/               # Transaction diffing
│   └── report/             # Report generation
├── web/                    # React 19 SPA (embedded into Go binary)
├── docs/                   # User documentation
├── specs/                  # System specifications (agent-oriented)
└── docker-compose.yml      # PostgreSQL service
```

### Key Conventions

- **Layer architecture**: Boundary → Domain → Infrastructure → Utility. See [`specs/architecture/layers.md`](specs/architecture/layers.md).
- **Interface ownership**: Consumers define interfaces (e.g., `api.RecordingStore`, `engine.CampaignStore`). `db` implements them.
- **Composition root**: All wiring in `internal/cli/serve.go:runServe()`.
- **Config**: Environment variables (`FFUUZZ_*`) then CLI flags. See `internal/config/config.go`.
- **Testing**: `go test ./... -race`. Use `sqlmock` for DB tests.
- **CI**: GitHub Actions run tests + linter on every push/PR.

### API Conventions

- All routes under `/api/v1/`
- Pagination: `limit` (default 50, max 50) + `offset` (default 0, max 1M)
- Error format: `{"error": "CODE", "message": "...", "request_id": "..."}`
- `X-Request-ID` auto-injected if not provided (format: `YYYYMMDD-UUIDv4`)
- SSE streaming at `/api/v1/campaigns/:id/stream`
- Empty lists: HTTP 204 No Content

### Frontend Conventions

- React 19 + TypeScript + Tailwind CSS v4 + daisyUI v5
- Client state: TanStack Query (`@tanstack/react-query`)
- Routing: React Router v7 (basename `/ui`)
- Build: Vite 6, output embedded into Go binary
- Theme: Light/dark toggle persisted via `data-theme` attribute
