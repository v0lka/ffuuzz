# ADR-001: Go Monolith with Embedded SPA

## Status
Accepted

## Context
FFUUZZ needs to be easy to distribute and run — a single binary with no external runtime dependencies beyond PostgreSQL. The frontend dashboard should be available without configuring a separate web server or CORS policies. The project must support both development (HMR for frontend) and production (single binary) workflows.

## Decision
Build a single Go binary that embeds the React SPA frontend via Go's `embed` package. The Go API server serves the frontend at `/ui/*` from an embedded `embed.FS`. In development, the Vite dev server runs separately with a proxy to the Go backend.

**Module structure**: Single Go module (`ffuuzz`) with all packages under `internal/`. No Go workspace or multi-module setup.

**Frontend embedding**: The `web/embed.go` file uses `//go:embed all:dist/*` to embed production build output. The `make build` target runs `npm ci && npm run build` then `go build`.

## Consequences

### Positive
- Single binary distribution: `./ffuuzz serve` runs everything
- No CORS configuration — frontend and API are served from the same origin
- No reverse proxy configuration needed for the frontend
- Go's `embed.FS` is read-only at compile time, eliminating runtime file serving concerns
- Development works naturally with Vite's proxy (`/api` → `localhost:8081`)

### Negative
- Frontend build must complete before Go build (`make build` has a dependency)
- The Go binary size increases by the frontend bundle (~500KB-1MB)
- Cannot update the frontend independently of the backend
- `embed.FS` paths must be accessed via `fs.Sub(web.DistFS, "dist")` to strip the `dist/` prefix
- Interface definitions are duplicated between `api` and `engine` packages (each defines its own store interfaces). This is accepted as the Dependency Inversion Principle pattern — the consumer owns the contract.

## Alternatives Considered

- **Separate frontend server (nginx/Caddy)**: Adds operational complexity. Requires CORS configuration. Makes distribution harder.
- **Go workspace with multiple modules**: Unnecessary for a single-team, single-repo project. Adds complexity without benefit.
- **API server on separate port from frontend**: Requires CORS. Complicates deployment.
- **Shared interface package**: Would create a package imported by both `api` and `db`, but no other domain logic, making it a dependency magnet. Rejected in favor of consumer-owned interfaces.
