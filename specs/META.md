# META

## Purpose

This spec system gives AI coding agents deterministic, up-to-date context about FFUUZZ's intended behaviour, interfaces, and architectural decisions. Agents should consult specs before making structural changes — the specs tell them *what the code is supposed to do*, so they don't have to reverse-engineer intent from implementation.

This document defines the rules all other spec files follow. Every agent working in this repository should read it first.

## File Organization

```
specs/
├── META.md                          ← you are here
├── INDEX.md                         ← task → spec mapping, full directory
├── WORKFLOW.md                      ← step-by-step development workflows
│
├── architecture/                    ← system-level concerns (layers, data flow, security)
│   ├── layers.md
│   ├── data-flow.md
│   └── security-model.md
│
├── domains/                         ← conceptual areas of functionality
│   ├── traffic-capture/
│   │   ├── README.md
│   │   ├── mitm.md
│   │   ├── recorder.md
│   │   └── endpoint.md
│   ├── fuzzing-engine/
│   │   ├── README.md
│   │   ├── engine.md
│   │   ├── mutate.md
│   │   ├── replayer.md
│   │   └── corpus.md
│   ├── anomaly-detection.md
│   ├── triage.md
│   ├── campaign-management.md
│   └── recordings.md
│
├── contracts/                       ← interface boundaries between layers
│   ├── cli-infrastructure.md
│   ├── api-engine.md
│   ├── api-db.md
│   ├── proxy-recorder.md
│   ├── engine-replayer.md
│   └── engine-stores.md
│
└── decisions/                       ← architecture decision records
    ├── _template.md
    ├── 001-go-monolith-embedded-spa.md
    ├── 002-postgresql-persistence.md
    ├── 003-mitm-tls-interception.md
    └── 004-mutation-fuzzing-approach.md
```

## Layer Architecture

FFUUZZ follows a strict 4-layer architecture:

```
┌─────────────────────────────────────────────┐
│  BOUNDARY                                    │
│  cli/  api/                                  │
│  ─ External interfaces, composition root     │
├─────────────────────────────────────────────┤
│  DOMAIN                                      │
│  model/  engine/  mutate/  anomaly/          │
│  triage/  corpus/  endpoint/                 │
│  ─ Pure business logic, no infrastructure    │
├─────────────────────────────────────────────┤
│  INFRASTRUCTURE                              │
│  db/  mitm/  recorder/  replayer/            │
│  store/  config/  metrics/                   │
│  ─ Technical capabilities, I/O              │
├─────────────────────────────────────────────┤
│  UTILITY                                     │
│  logging/  httputil/  diff/  report/         │
│  ─ Zero-dep cross-cutting helpers            │
└─────────────────────────────────────────────┘
```

**Import rules** (invariants):
- **model/** has zero internal dependencies. Every other package may import it.
- **Utility** packages (`logging`, `httputil`, `diff`, `report`) have zero internal dependencies.
- **Domain** packages depend only on `model` and utility packages.
- **Infrastructure** packages depend on `model`, utility, and domain packages at their interfaces.
- **Boundary** packages depend on everything and wire the application together.
- Reverse dependencies (infrastructure → boundary, domain → infrastructure, utility → domain) are forbidden.

The canonical composition root is `internal/cli/serve.go:runServe()`. All dependency wiring for the `serve` command happens there.

## Spec Types and Templates

### 1. Architecture Spec (`architecture/*.md`)

Documents a system-level concern that spans multiple domains or layers.

**Template:**

```markdown
# Title (short, descriptive)

## Overview
One paragraph explaining what this concern is about and why it matters.

## Diagram
ASCII diagram showing the flow or relationships.

## Components
List of packages, modules, or logical components involved, with their roles.

## Invariants
- Statement of guaranteed behaviour (format: "X always happens before Y", "Z is never null")
- Each invariant must be falsifiable

## Anti-patterns
- Common mistakes specific to this codebase
- What NOT to do and why

## Related
- [`specs/architecture/other.md`] — short description
- [`specs/domains/something.md`] — short description
```

### 2. Domain README (`domains/<name>/README.md`)

Overview of a domain with multiple components. Each component gets its own detail file.

**Template:**

```markdown
# <Domain Name>

## Overview
One sentence about the domain's responsibility. 2–3 sentences explaining the problem it solves and how.

## Key Files

| File | Role |
|------|------|
| `internal/foo/foo.go` | What this file does |

## Core Types

\```go
// Type definition from actual code
\```

## Flow
ASCII diagram showing the happy-path flow through the domain.

## Components
List of sub-component spec files in this domain directory.

## Invariants
- Falsifiable statement
- Falsifiable statement

## Configuration
Relevant config keys, env vars, and their effects.
```

### 3. Domain Detail (`domains/<name>/<component>.md`)

Deep-dive into a single component within a domain.

**Template:**

```markdown
# <Component Name>

## Responsibility
One sentence.

## Key Types

\```go
// Interface or struct definition
\```

## Public API

### `FunctionName(args) (returns)`
What it does. When to call it.

### `MethodName(args) (returns)`
What it does. Preconditions. Side effects.

## Internal Flow
ASCII or numbered-step diagram tracing the main execution path through the code.

## Invariants
- Falsifiable statement

## Dependencies
| Package | Used for |
|---------|----------|
| `internal/foo` | Why |

## Edge Cases
- Scenario → expected behaviour
```

### 4. Contract Spec (`contracts/<sender>-<receiver>.md`)

Documents the interface boundary between two layers/modules.

**Template:**

```markdown
# <Sender> → <Receiver>

## Overview
Which layer consumes which, and why.

## Interfaces

\```go
// Interface definition from actual code (sender-side)
\```

## Implementations
| Package | Type | Notes |
|---------|------|-------|
| `internal/db` | `db.CampaignStore` | PostgreSQL-backed |

## Initialization
How the interface is wired at startup. Reference the file and line.

## Data Flow
ASCII or step-by-step showing how data crosses this boundary.

## Breaking Change Checklist
- [ ] Are all implementations updated?
- [ ] Are callers compatible with new signatures?
- [ ] Is error handling maintained?
```

### 5. ADR (`decisions/NNN-title.md`)

Architecture Decision Record.

**Template** (see `decisions/_template.md`):

```markdown
# ADR-NNN: Title

## Status
Proposed | Accepted | Deprecated | Superseded by ADR-NNN

## Context
The problem we were solving. Relevant constraints. Technology landscape.

## Decision
What we chose and why.

## Consequences
What got easier. What got harder. Trade-offs we accept.
```

## Conventions

### Invariants

Invariants are **affirmative, falsifiable statements** about guaranteed behaviour:
- "X always happens before Y"
- "Z is never null when W is true"
- "A is called at most once per request"

Not observations ("we use X for Y") or aspirations ("we should do X"). Invariants must be verifiable by reading the code or tests.

### Terminology

| Term | Definition |
|------|------------|
| **Recording / Session** | A captured HTTP conversation with one or more exchanges. Model type: `model.RecordingSession`. |
| **Exchange** | A single request/response pair within a recording. Model type: `model.Exchange`. |
| **Campaign** | A fuzzing run with target, mutation config, anomaly detectors, and triage settings. Model type: `model.Campaign`. |
| **Seed** | A recording session used as input to the fuzzing engine. Workers mutate exchanges from seeds. |
| **Baseline** | P50 latency per endpoint computed from recorded traffic. Used by latency detector. |
| **Finding** | A confirmed or unconfirmed anomaly discovered during fuzzing. Model type: `model.Finding`. |
| **Artifact** | A JSON file containing the full payload to reproduce a finding. Stored in the artifact directory. |
| **Endpoint** | A normalized HTTP path pattern (e.g., `/api/users/{_}`). Path parameters are collapsed to `{_}`. |
| **MITM Proxy** | The man-in-the-middle HTTP/HTTPS proxy that intercepts traffic for recording. Implementation: `internal/mitm`. |
| **Control API** | The REST API for managing recordings, campaigns, and findings. Implementation: `internal/api`. |
| **Worker** | A goroutine that continuously takes seed tasks, mutates them, replays them, and detects anomalies. |
| **Triage** | The process of confirming, deduplicating, and minimizing findings. Implementation: `internal/triage`. |
| **Reproduce Worker** | A background goroutine that picks up enqueued reproduce jobs and replays the finding N times. |

### Cross-References

Link to other spec files using relative paths from the specs root:
- `[layers.md](architecture/layers.md)` — from a file in `specs/` root
- `[engine.md](engine.md)` — from a sibling file in the same directory
- `[README.md](../traffic-capture/README.md)` — from a file in `domains/fuzzing-engine/`

When referencing source code, use the pattern `[file.go](internal/package/file.go:line)` for the first reference in a section, then `file.go:L` for subsequent references.

### Key Files: Path Verification

All file paths listed in "Key Files" tables must point to actual files in the repository. Verify each path before committing a spec.

## Update Rules

### When to Update a Spec

| Trigger | Action |
|---------|--------|
| Adding a new package/module | Create domain detail or decide if it fits existing domain. Update domain README. |
| Changing an interface | Update the contract spec. Update all domain specs referencing that interface. |
| Adding a new layer dependency | Update `architecture/layers.md`. |
| Changing the startup wiring | Update `architecture/data-flow.md` and relevant contract specs. |
| Making a structural architectural decision | Create a new ADR in `decisions/`. |
| Renaming or moving a package | Update all spec files referencing it. Verify cross-references. |

### Before Merging a Spec Change

- [ ] All cross-references resolve to existing files
- [ ] All "Key Files" paths point to actual source files
- [ ] Invariants are stated affirmatively and are falsifiable
- [ ] ASCII diagrams align in monospace (no jagged edges)
- [ ] No stale terminology (check the glossary)

### Spec Versioning

Specs do not have version numbers. They are living documents that always reflect the current state of the codebase. The git history provides versioning — use `git log -- specs/` to see what changed and when.

## Relationship to Other Documentation

- **`README.md`** — project overview for humans. Less detailed than specs, more marketing-oriented.
- **`docs/`** — user-facing guides, educational content, formal specification in Russian. Not agent-oriented.
- **`specs/`** — agent-oriented system documentation. Deterministic, structured, code-derived.
- **`.qoder/rules/`** — agent behaviour rules (how to work). Not about what the system does.

When information exists in both `docs/` and `specs/`, specs are authoritative for agents because they are code-derived and maintained alongside code changes.
