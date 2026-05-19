# INDEX

Start here to find the spec relevant to your task. Scan the **Task → Spec** table, then follow the links.

## Task → Spec

| Task | Spec(s) to read |
|------|-----------------|
| Understand the overall architecture | [`architecture/layers.md`](architecture/layers.md) |
| Trace a request through the system | [`architecture/data-flow.md`](architecture/data-flow.md) |
| Add a new anomaly detector | [`domains/anomaly-detection.md`](domains/anomaly-detection.md) |
| Add a new mutation strategy | [`domains/fuzzing-engine/mutate.md`](domains/fuzzing-engine/mutate.md) |
| Add a new API endpoint | [`contracts/api-db.md`](contracts/api-db.md), [`contracts/api-engine.md`](contracts/api-engine.md) |
| Change campaign lifecycle, grouping | [`domains/campaign-management.md`](domains/campaign-management.md), [`domains/fuzzing-engine/engine.md`](domains/fuzzing-engine/engine.md) |
| Modify MITM proxy behaviour | [`domains/traffic-capture/mitm.md`](domains/traffic-capture/mitm.md), [`architecture/security-model.md`](architecture/security-model.md) |
| Modify recording/storage | [`domains/traffic-capture/recorder.md`](domains/traffic-capture/recorder.md), [`contracts/proxy-recorder.md`](contracts/proxy-recorder.md) |
| Modify endpoint normalization | [`domains/traffic-capture/endpoint.md`](domains/traffic-capture/endpoint.md) |
| Modify finding triage, severity, categorization, grouping | [`domains/triage.md`](domains/triage.md) |
| Modify HTTP replay logic | [`domains/fuzzing-engine/replayer.md`](domains/fuzzing-engine/replayer.md), [`contracts/engine-replayer.md`](contracts/engine-replayer.md) |
| Modify TLS/cert management | [`architecture/security-model.md`](architecture/security-model.md) |
| Change startup wiring, background loops | [`contracts/cli-infrastructure.md`](contracts/cli-infrastructure.md) |
| Change database schema | [`contracts/api-db.md`](contracts/api-db.md), [`contracts/engine-stores.md`](contracts/engine-stores.md), [`decisions/002-postgresql-persistence.md`](decisions/002-postgresql-persistence.md) |
| Add a new package/module | [`architecture/layers.md`](architecture/layers.md), [`META.md`](META.md) |
| Understand a design decision | [`decisions/`](decisions/) directory |
| Work with the frontend | [`decisions/001-go-monolith-embedded-spa.md`](decisions/001-go-monolith-embedded-spa.md) |

## Dependency Graph

```
┌─────────────────┐     ┌─────────────────┐
│    CLI (serve)   │────►│   API (Gin)     │
│  Composition Root│     │ REST + SPA + SSE│
└────────┬────────┘     └────────┬────────┘
         │                       │
         │              ┌────────┼────────┐
         │              ▼        ▼        ▼
         │        ┌─────────┐ ┌──────┐ ┌─────────┐
         │        │ Campaign│ │Finding│ │Recording│  ← API stores
         │        │  Store  │ │ Store │ │  Store  │
         │        └────┬────┘ └──┬───┘ └────┬────┘
         │             │         │          │
         │             ▼         ▼          ▼
         │        ┌────────────────────────────────┐
         │        │         PostgreSQL              │
         │        │  (migrations, sqlx, JSONB)      │
         │        └───────────────┬────────────────┘
         │                        │
         ▼                        ▼
┌─────────────────┐     ┌─────────────────┐
│   Engine         │     │  MITM Proxy     │
│ Campaign Lifecycle│     │ (:8080)         │
│ Worker Pool      │     │                 │
│ Rate Limiter     │     └────────┬────────┘
│ Reproduce Worker │              │
└────────┬─────────┘              ▼
         │              ┌─────────────────┐
         │              │   Recorder      │
         │              │ DBRecorder      │
         │              │ JSONL File      │
         │              └────────┬────────┘
         │                       │
         ▼                       ▼
┌─────────────────────────────────────────┐
│  Domain Packages                         │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ │
│  │  mutate  │ │ anomaly  │ │  triage  │ │
│  └──────────┘ └──────────┘ └──────────┘ │
│  ┌──────────┐ ┌──────────┐              │
│  │  corpus  │ │ endpoint │              │
│  └──────────┘ └──────────┘              │
├─────────────────────────────────────────┤
│  Infrastructure                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ │
│  │  config  │ │  store   │ │ metrics  │ │
│  │(env+flag+│ │(cert LRU)│ │(prom.)   │ │
│  │ dotenv)  │ │          │ │          │ │
│  └──────────┘ └──────────┘ └──────────┘ │
│  ┌──────────┐ ┌──────────┐              │
│  │ replayer │ │   llm    │              │
│  │          │ │ provider │              │
│  └──────────┘ └──────────┘              │
├─────────────────────────────────────────┤
│  Utility                                 │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ │
│  │ logging  │ │ httputil │ │  diff    │ │
│  │ (zerolog)│ │ (helpers)│ │          │ │
│  └──────────┘ └──────────┘ └──────────┘ │
│  ┌──────────┐                           │
│  │  report  │                           │
│  └──────────┘                           │
└─────────────────────────────────────────┘
```

## Directory Listing

### Meta
- [`META.md`](META.md) — Spec system rules, templates, conventions, update workflow
- [`INDEX.md`](INDEX.md) — This file (task → spec mapping, dependency graph, directory listing)
- [`WORKFLOW.md`](WORKFLOW.md) — Step-by-step development workflows for common tasks

### Architecture
- [`architecture/layers.md`](architecture/layers.md) — 4-layer hierarchy (Boundary → Domain → Infrastructure → Utility), import rules
- [`architecture/data-flow.md`](architecture/data-flow.md) — End-to-end request lifecycle: proxy → record → fuzz → triage → persist
- [`architecture/security-model.md`](architecture/security-model.md) — TLS interception, certificate generation, variable extraction safety

### Domains

**Traffic Capture** — Intercept, record, and normalize HTTP traffic
- [`domains/traffic-capture/README.md`](domains/traffic-capture/README.md) — Domain overview
- [`domains/traffic-capture/mitm.md`](domains/traffic-capture/mitm.md) — MITM proxy (CONNECT, TLS interception)
- [`domains/traffic-capture/recorder.md`](domains/traffic-capture/recorder.md) — Recorder interface, DBRecorder, JSONL recorder
- [`domains/traffic-capture/endpoint.md`](domains/traffic-capture/endpoint.md) — Path normalization, trie collapsing, Merger interface

**Fuzzing Engine** — Campaign orchestration, mutation, replay
- [`domains/fuzzing-engine/README.md`](domains/fuzzing-engine/README.md) — Domain overview
- [`domains/fuzzing-engine/engine.md`](domains/fuzzing-engine/engine.md) — Engine: campaign lifecycle, rate limiting, reproduce worker
- [`domains/fuzzing-engine/mutate.md`](domains/fuzzing-engine/mutate.md) — Mutation pipeline: URI, header, JSON, param, sequence, primitive
- [`domains/fuzzing-engine/replayer.md`](domains/fuzzing-engine/replayer.md) — HTTP replay, WorkerContext, variable substitution
- [`domains/fuzzing-engine/corpus.md`](domains/fuzzing-engine/corpus.md) — Seed loading, P50 baseline computation

**Single-File Domains**
- [`domains/anomaly-detection.md`](domains/anomaly-detection.md) — Detector interface, 4 detectors, MultiDetector
- [`domains/triage.md`](domains/triage.md) — Dedup (signatures), confirmation, minimization (delta-debugging), severity scoring, OWASP categorization, vulnerability grouping, LLM-assisted analysis
- [`domains/campaign-management.md`](domains/campaign-management.md) — Campaign CRUD, status lifecycle, SSE streaming
- [`domains/recordings.md`](domains/recordings.md) — Recording import/export, tree, management

### Contracts
- [`contracts/cli-infrastructure.md`](contracts/cli-infrastructure.md) — CLI → config, DB, cert store wiring
- [`contracts/api-engine.md`](contracts/api-engine.md) — API → Engine boundary
- [`contracts/api-db.md`](contracts/api-db.md) — API → DB store interfaces
- [`contracts/proxy-recorder.md`](contracts/proxy-recorder.md) — MITM → Recorder interface
- [`contracts/engine-replayer.md`](contracts/engine-replayer.md) — Engine → Replayer, SessionReplayer interface
- [`contracts/engine-stores.md`](contracts/engine-stores.md) — Engine → DB store interfaces

### Decisions
- [`decisions/_template.md`](decisions/_template.md) — ADR template
- [`decisions/001-go-monolith-embedded-spa.md`](decisions/001-go-monolith-embedded-spa.md) — Go monolith with embedded React SPA
- [`decisions/002-postgresql-persistence.md`](decisions/002-postgresql-persistence.md) — PostgreSQL with sqlx and JSONB
- [`decisions/003-mitm-tls-interception.md`](decisions/003-mitm-tls-interception.md) — On-the-fly TLS certificate generation
- [`decisions/004-mutation-fuzzing-approach.md`](decisions/004-mutation-fuzzing-approach.md) — Mutation-based fuzzing pipeline

### Infrastructure
- [`infrastructure/config.md`](infrastructure/config.md) — Application configuration: 22 env vars, 13 CLI flags, loading priority chain, `DefaultConfig()`
- [`infrastructure/llm-providers.md`](infrastructure/llm-providers.md) — LLM provider implementations: OpenAI (structured JSON output) and Anthropic (regex JSON extraction)
- [`infrastructure/metrics.md`](infrastructure/metrics.md) — Prometheus metrics: 11 metrics in a custom registry (non-global)

### Utility
- [`utility/httputil.md`](utility/httputil.md) — HTTP helpers: hop-by-hop headers, LimitedBuffer, TeeReadCloser, request IDs, server construction
- [`utility/diff.md`](utility/diff.md) — Structural diff between two TxRecords (URL + status comparison)
- [`utility/report.md`](utility/report.md) — Aggregate summary: method/status/host breakdowns from recorded transactions
