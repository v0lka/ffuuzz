# Layers

## Overview

FFUUZZ follows a strict 4-layer architecture where dependencies flow downward: Boundary → Domain → Infrastructure → Utility. The `model` package is the pure core with zero internal dependencies. All wiring happens in a single composition root (`internal/cli/serve.go`), and each layer owns the interfaces it consumes.

## Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│  BOUNDARY                                                           │
│                                                                     │
│  ┌──────────┐    ┌──────────────────────────────────────────────┐   │
│  │   CLI    │    │                  API                         │   │
│  │          │    │  ┌────────────┐  ┌──────────┐  ┌─────────┐  │   │
│  │  serve   │    │  │ Recordings │  │ Campaigns │  │Findings │  │   │
│  │  proxy   │────│  │  handlers  │  │ handlers  │  │handlers │  │   │
│  │  record  │    │  └────────────┘  └──────────┘  └─────────┘  │   │
│  └────┬─────┘    │  ┌──────┐  ┌──────┐  ┌───────────┐         │   │
│       │          │  │ SSE  │  │ SPA  │  │ Middleware │         │   │
│       │          │  │stream│  │serve │  │(reqID,log) │         │   │
│       │          │  └──────┘  └──────┘  └───────────┘         │   │
│       │          └──────────────────────────────────────────────┘   │
│       │                              │                              │
├───────┼──────────────────────────────┼──────────────────────────────┤
│       │         DOMAIN               │                              │
│       │                              │                              │
│  ┌────┴─────────┐  ┌──────────┐  ┌──┴──────────┐  ┌────────────┐   │
│  │    engine    │  │  mutate  │  │   anomaly   │  │   triage   │   │
│  │  campaign    │  │ pipeline │  │  detectors  │  │ confirm    │   │
│  │  lifecycle   │  │ urimut   │  │  multi-det  │  │ minimize   │   │
│  └────┬─────────┘  │ headers  │  └─────────────┘  │ signature  │   │
│       │            │ jsonbody │                   └────────────┘   │
│  ┌────┴─────────┐  │ params   │  ┌──────────┐  ┌────────────┐      │
│  │   corpus     │  │ sequence │  │ endpoint │  │   model    │      │
│  │ seeds+bases  │  │ primitive│  │ resolver │←─│ all types  │      │
│  └──────────────┘  └──────────┘  │ trie col │  └──────┬─────┘      │
│                                  └──────────┘         │            │
├──────────────────────────────────────────────────────┼────────────┤
│                     INFRASTRUCTURE                    │            │
│                                                      │            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────┐│            │
│  │    db    │  │   mitm   │  │ recorder │  │replay ││            │
│  │ postgres │  │ CONNECT  │  │ DB+JSONL │  │ HTTP  │◄────────────┤
│  │ stores   │  │ TLS MITM │  │ norm path│  │ state ││            │
│  └──────────┘  └──────────┘  └──────────┘  └───────┘│            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐            │            │
│  │  store   │  │  config  │  │ metrics  │            │            │
│  │ cert LRU │  │ env+flag │  │prometheus│            │            │
│  └──────────┘  └──────────┘  └──────────┘            │            │
├──────────────────────────────────────────────────────┼────────────┤
│                     UTILITY                          │            │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌───────┐│            │
│  │ logging  │  │ httputil │  │  diff    │  │report ││            │
│  │ zerolog  │  │ headers  │  │ tx cmp   │  │ stats ││            │
│  │ factory  │  │ server   │  │          │  │       ││            │
│  └──────────┘  └──────────┘  └──────────┘  └───────┘│            │
└──────────────────────────────────────────────────────┴────────────┘
```

## Components

| Package | Layer | Role |
|---------|-------|------|
| `internal/model` | Domain | Pure data types: `Campaign`, `Finding`, `Exchange`, `RecordingSession`, config structs, enums. Zero internal dependencies. |
| `internal/cli` | Boundary | CLI entry point. Parses subcommands (`serve`, `proxy`, `record`). `runServe()` is the composition root. |
| `internal/api` | Boundary | Gin-based REST API. Owns store interfaces (`RecordingStore`, `CampaignStore`, etc.). Serves embedded SPA and SSE streams. |
| `internal/engine` | Domain | Campaign lifecycle orchestration, worker pool, rate limiting, reproduce worker. Owns `CampaignStore`, `FindingStore`, `ArtifactStore` interfaces. |
| `internal/mutate` | Domain | Mutation pipeline: URI, header, JSON body, param, sequence, and primitive mutators. Statistics-agnostic. |
| `internal/anomaly` | Domain | Pluggable detectors (`TimeoutDetector`, `ServerErrorDetector`, `LatencyDetector`, `RegexDetector`) composed via `MultiDetector`. |
| `internal/triage` | Domain | Finding dedup (signatures), confirmation (N-replay), minimization (session + JSON body delta-debugging). |
| `internal/corpus` | Domain | Seed loading from campaign recording IDs. P50 baseline computation per endpoint. |
| `internal/endpoint` | Domain | Path normalisation (regex heuristic) and trie-based statistical collapse detection. `Merger` interface for DB-backed recording grouping. |
| `internal/db` | Infrastructure | PostgreSQL stores: `RecordingStore`, `CampaignStore`, `FindingStore`, `ArtifactStore`. Embedded migrations via `golang-migrate`. |
| `internal/mitm` | Infrastructure | MITM HTTP/HTTPS proxy. CONNECT tunnelling, TLS interception, traffic forwarding. Calls `recorder.Recorder`. |
| `internal/recorder` | Infrastructure | Recording pipeline: `Recorder` interface, `DBRecorder` (normalised paths, grouping), JSONL file recorder. |
| `internal/replayer` | Infrastructure | HTTP replay client. `Replayer` sends exchanges to target. `WorkerContext` manages cookies, variables, extraction rules. |
| `internal/store` | Infrastructure | TLS certificate management: root CA generation, leaf cert LRU cache, disk persistence, `singleflight` dedup. |
| `internal/config` | Infrastructure | Configuration loading from env vars (`FFUUZZ_*`) and CLI flags. `DefaultConfig()` with production-safe defaults. |
| `internal/metrics` | Infrastructure | Prometheus metrics in a custom registry. Counters, histograms for tests, findings, cert cache, endpoints. |
| `internal/llm` | Infrastructure | LLM provider abstraction: OpenAI and Anthropic backends. Factory (`NewProvider`) supports graceful degradation when disabled. |
| `internal/logging` | Utility | Zerolog factory. Context helpers for request/campaign/recording IDs. Zero internal deps. |
| `internal/httputil` | Utility | HTTP helpers: hop-by-hop header removal, limited body buffers, tee readers, request IDs, server construction. |
| `internal/diff` | Utility | Structural diff between two `TxRecord`s (URL + status comparison). |
| `internal/report` | Utility | Aggregation: summary stats from `[]TxRecord`. |

## Import Rules

These are invariants enforced by the Go compiler (all packages are `internal/`):

1. **`model` imports nothing from the project.** Every other package may import `model`.
2. **Utility packages** (`logging`, `httputil`, `diff`, `report`) import no other `ffuuzz` packages.
3. **Domain packages** may import `model`, utility packages, and other domain packages. Domain packages must NOT import infrastructure or boundary packages.
4. **Infrastructure packages** may import `model`, utility, and domain packages. They must NOT import boundary packages.
5. **Boundary packages** (`cli`, `api`) may import anything. They are the root of the dependency graph.
6. **Interface ownership**: each layer defines the interfaces it needs. `api` defines `api.RecordingStore`; `engine` defines `engine.CampaignStore`; `db` implements both. The implementation package (`db`) does NOT export the interfaces.

## Invariants

- `model` is the only package that may be imported by every other package. No other package has this property.
- All application wiring happens in exactly one function: `CLI.runServe()` in `internal/cli/serve.go`. No other code creates infrastructure dependencies or connects packages.
- Interface definitions live in the consuming package (e.g. `api.RecordingStore`, `engine.CampaignStore`), never in the implementing package (`db`).
- Infrastructure packages (`db`, `mitm`, `store`, `config`) never import each other. Cross-infrastructure dependencies go through domain interfaces.
- The `config` package is read-once at startup. No package reads config after initialisation.
- `metrics.Registry()` is a custom `prometheus.Registry`, not the global default.

## Anti-patterns

- **DO NOT** add an import of `internal/cli` or `internal/api` to any non-boundary package. This creates a reverse dependency.
- **DO NOT** define interfaces in `internal/db` and export them. Interfaces belong to the consumer (`api`, `engine`).
- **DO NOT** create new wiring functions outside `internal/cli/serve.go`. The composition root is the single source of truth for dependency injection.
- **DO NOT** import `internal/config` from domain packages. Domain logic must be configurable through parameters, not by reading config directly.
- **DO NOT** add new packages that bypass the layer structure. Every new package must fit into Boundary, Domain, Infrastructure, or Utility.

## Related

- [`data-flow.md`](data-flow.md) — end-to-end request lifecycle across layers
- [`security-model.md`](security-model.md) — TLS interception and certificate management
- [`contracts/cli-infrastructure.md`](../contracts/cli-infrastructure.md) — how the CLI wires infrastructure at startup

## Related

- [`data-flow.md`](data-flow.md) — end-to-end request lifecycle across layers
- [`security-model.md`](security-model.md) — TLS interception and certificate management
- [`contracts/cli-infrastructure.md`](../contracts/cli-infrastructure.md) — how the CLI wires infrastructure at startup
