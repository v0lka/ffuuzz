# Data Flow

## Overview

FFUUZZ operates in two distinct phases — **record** and **fuzz** — connected by a shared PostgreSQL database. The record phase captures real HTTP traffic through an MITM proxy. The fuzz phase mutates that traffic, replays it against the target, detects anomalies, and triages findings. A background reproduce worker validates and persists confirmed findings.

## Diagram

```
                             RECORD PHASE
    ┌──────────┐
    │  Client  │ (browser, curl, app)
    └────┬─────┘
         │ HTTP/HTTPS (proxy configured)
         ▼
┌───────────────────────────────────────────────────────┐
│  MITM Proxy (:8080)                 internal/mitm    │
│                                                       │
│  ┌─────────────────────────────────────────────────┐  │
│  │ HTTP: read request → forward → record response  │  │
│  │ HTTPS: CONNECT hijack → TLS MITM → sign cert    │  │
│  │        → tunnel → read → forward → record       │  │
│  └───────────────────────┬─────────────────────────┘  │
└──────────────────────────┼────────────────────────────┘
                           │ Recorder.Record(TxRecord)
                           ▼
┌───────────────────────────────────────────────────────┐
│  Recorder                      internal/recorder     │
│                                                       │
│  DBRecorder:                                          │
│  1. Parse URL → scheme, host, port, path              │
│  2. NormalizePath(path) → replace params with {_}    │
│  3. resolver.ObservePath(origin, normPath)            │
│  4. Convert TxRecord → model.Exchange                 │
│  5. FindOrAppend(origin, exchange)                    │
└───────────────────────┬───────────────────────────────┘
                        │
                        ▼
┌───────────────────────────────────────────────────────┐
│  PostgreSQL                      internal/db         │
│                                                       │
│  Tables:                                              │
│  ┌──────────────────┐  ┌──────────────────────────┐  │
│  │ recording_sessions│  │ exchanges (session_id FK) │  │
│  │ - id             │  │ - request_id              │  │
│  │ - target_scheme  │  │ - method                  │  │
│  │ - target_host    │  │ - path (normalised)       │  │
│  │ - target_port    │  │ - query                   │  │
│  │ - target_path    │  │ - req_headers (JSONB)     │  │
│  │ - created_at     │  │ - req_body_b64            │  │
│  │ - entry_count    │  │ - resp_status             │  │
│  └──────────────────┘  │ - resp_body_b64           │  │
│                        │ - duration_ms             │  │
│                        │ - started_at              │  │
│                        └──────────────────────────┘  │
└───────────────────────────────────────────────────────┘


                              FUZZ PHASE
    ┌──────────┐
    │  API/UI  │ POST /api/v1/campaigns/:id/start
    └────┬─────┘
         │
         ▼
┌───────────────────────────────────────────────────────┐
│  Engine::StartCampaign          internal/engine      │
│                                                       │
│  1. CREATED → STARTING (optimistic lock)              │
│  2. corpus.GetSeeds(campaignID)  → []RecordingSession │
│  3. corpus.ComputeBaseline(seeds) → map[endpoint]P50  │
│  4. STARTING → RUNNING                                │
│  5. spawn goroutine: runCampaign()                    │
└───────────────────────┬───────────────────────────────┘
                        │ runCampaign goroutine
                        ▼
┌───────────────────────────────────────────────────────┐
│  Task Generator + Worker Pool   internal/engine      │
│                                                       │
│  ┌─────────────────────────────────────────────────┐  │
│  │ Task Generator (single goroutine)                │  │
│  │                                                  │  │
│  │  loop:                                           │  │
│  │    check ctx.Done(), max_tests, duration_sec     │  │
│  │    limiter.Acquire()  (token bucket)             │  │
│  │    pick random seed session                      │  │
│  │    taskCh ← SeedTask{session, mutationSeed}      │  │
│  └───────────────────────┬─────────────────────────┘  │
│                          │                             │
│         ┌────────────────┼────────────────────┐       │
│         ▼                ▼                     ▼       │
│  ┌────────────┐  ┌────────────┐        ┌────────────┐ │
│  │  Worker 0  │  │  Worker 1  │  ...   │ Worker N-1 │ │
│  │            │  │            │        │            │ │
│  │  loop:     │  │            │        │            │ │
│  │ 1. mutate  │  │            │        │            │ │
│  │ 2. replay  │  │            │        │            │ │
│  │ 3. detect  │  │            │        │            │ │
│  │ 4. triage  │  │            │        │            │ │
│  │ 5. persist │  │            │        │            │ │
│  └────────────┘  └────────────┘        └────────────┘ │
└───────────────────────────────────────────────────────┘

                    Worker Loop Detail

  ┌──────────────────────────────────────────────────┐
  │ SeedTask arrives on taskCh                        │
  └──────────────────┬───────────────────────────────┘
                     ▼
  ┌──────────────────────────────────────────────────┐
  │ 1. MUTATE                  internal/mutate       │
  │    pipeline.Mutate(exchange, seed)                │
  │    ─ Applies URI, header, JSON, param mutations  │
  │    ─ Returns MutationResult with mutated exchange │
  └──────────────────┬───────────────────────────────┘
                     ▼
  ┌──────────────────────────────────────────────────┐
  │ 2. REPLAY                  internal/replayer     │
  │    replayer.ReplayExchange(ctx, ex, baseURL, wctx)│
  │    ─ Sends mutated HTTP request to target         │
  │    ─ wctx.ApplySubstitutions() {{var}} → value    │
  │    ─ wctx.ExtractVariables() from response        │
  │    ─ wctx.UpdateCookies() from Set-Cookie         │
  │    ─ Returns ExchangeResult{status, body, dur}   │
  └──────────────────┬───────────────────────────────┘
                     ▼
  ┌──────────────────────────────────────────────────┐
  │ 3. DETECT                  internal/anomaly      │
  │    detector.Detect(ex, result, baseline, cfg)     │
  │    ─ TimeoutDetector: result.Err is timeout       │
  │    ─ ServerErrorDetector: status >= 500           │
  │    ─ LatencyDetector: duration > baseline * mult  │
  │    ─ RegexDetector: body matches pattern          │
  │    ─ Returns []AnomalyHit (may be empty)          │
  └──────────────────┬───────────────────────────────┘
                     ▼
  ┌──────────────────────────────────────────────────┐
  │ 4. TRIAGE                   internal/triage      │
  │    For each AnomalyHit:                           │
  │    ─ signature = triager.Signature(hit)           │
  │    ─ exists = findings.ExistsBySignature(sig)      │
  │    ─ if exists: skip (dedup)                      │
  │    ─ if !exists:                                  │
  │      · severity = ScoreSeverity(type,...,rep=0)   │
  │      · category = CategorizeFinding(type,...)     │
  │      · create Finding{Severity, OWASPCategory}    │
  │      · confirmed, rep, err = triager.Confirm(N)   │
  │      · reproducibility = rep / runs               │
  │      · severity = ScoreSeverity(rep=reproduc.)    │
  │      · triager.MinimizeSession(hit, replayer)     │
  │      · ct = GetContentType(ex)                    │
  │      · route minimizer: JSON/params/XML/multipart │
  └──────────────────┬───────────────────────────────┘
                     ▼
  ┌──────────────────────────────────────────────────┐
  │ 5. PERSIST                  internal/db          │
  │    ─ findings.Create(Finding)                     │
  │    ─ if confirmed: findings.UpdateStatus(id, CONFIRMED)│
  │    ─ update finding severity + reproducibility    │
  │    ─ artifacts.Create(Artifact) ← JSON payload    │
  │    ─ campaigns.IncrementStats(1, findingDelta)    │
  │    ─ metrics.FindingsTotal.Inc()                  │
  └──────────────────┬───────────────────────────────┘
                     ▼
                   (loop)
```

## Phases

### Record Phase

1. **MITM Proxy** (`internal/mitm/mitm.go:ServeHTTP`) intercepts HTTP/HTTPS requests.
   - HTTP: reads the request, forwards it to the origin server, records request + response.
   - HTTPS: receives CONNECT, hijacks the connection, performs TLS handshake with an on-the-fly generated certificate, then tunnels plaintext HTTP inside the encrypted channel.
2. **Recorder** (`internal/recorder/recorder.go`) receives `TxRecord` from the proxy.
   - `DBRecorder` parses the URL, normalises the path (replacing dynamic segments with `{_}`), observes the path in the `endpoint.Resolver`, and stores the exchange via `db.RecordingStore.FindOrAppend`.
3. **Endpoint Resolver** (`internal/endpoint/resolver.go`) builds a path segment trie for each origin. When a path segment has high cardinality across recordings, it collapses it into `{_}` and triggers a DB merge via the `Merger` interface.

### Fuzz Phase

1. **Campaign Start** (`internal/engine/engine.go:StartCampaign`) transitions campaign status CREATED → STARTING, loads recording seeds via `corpus.Manager`, computes P50 latency baselines per endpoint, transitions to RUNNING, and spawns a goroutine for the campaign.
2. **Task Generator** generates `SeedTask` values (random seed session + random mutation seed) and pushes them to a buffered channel. It obeys `max_tests`, `duration_sec`, context cancellation, and rate limiting.
3. **Worker Pool** (N workers, default 4) consumes tasks from the channel. Each worker:
   - **Mutates** the seed exchange via `mutate.Pipeline.Mutate()`.
   - **Replays** the mutated exchange against the target via `replayer.Replayer.ReplayExchange()`.
   - **Detects** anomalies via `anomaly.MultiDetector.Detect()`.
   - **Triages** findings: deduplicates by signature, assigns severity and OWASP category, confirms by re-sending N times, re-scores severity with actual reproducibility, minimizes the session and body (JSON/query-params/XML/multipart).
   - **Persists** findings and artifacts to PostgreSQL.
4. **SSE Streaming** (`internal/api/` streams campaign stats (tests done, findings per type, last activity) to connected clients.
5. **LLM Analysis (API-initiated)** — Two endpoints allow on-demand LLM analysis: `POST /api/v1/findings/:id/analyze` analyzes a single finding, and `POST /api/v1/campaigns/:id/analyze` batch-analyzes all unconfirmed findings in a campaign. Both return HTTP 503 when LLM is disabled.
6. **Reproduce Worker** (`internal/engine/reproduce.go`) polls for enqueued reproduce jobs and replays the finding's artifact session against the target.
6. **Vulnerability Grouping** — At campaign stop, confirmed findings are grouped by type/mutation/endpoint/status-range and assigned a shared `GroupID`. A background goroutine also groups periodically every 15s.

### Shutdown Phase

1. Shutdown signal received (SIGINT/SIGTERM).
2. API server shuts down (stops accepting new requests, drains in-flight requests).
3. All running campaigns are stopped (`Engine.StopAll`).
4. Proxy server shuts down.
5. Database connection is closed (via deferred `database.Close()`).

## Invariants

- Recording and fuzzing phases share data only through PostgreSQL. The MITM proxy never directly feeds the fuzzing engine.
- Seed loading happens synchronously during `StartCampaign`. If no seeds exist, the campaign transitions to FAILED before workers are spawned.
- The task generator is a single goroutine. Workers are N goroutines consuming from a shared channel. No worker creates its own tasks.
- Rate limiting uses a token-bucket algorithm. Workers block on `limiter.Acquire(ctx)` before each mutation.
- Anomaly detection results are processed per-exchange, not batched. Each mutated exchange triggers a full detect→triage→persist cycle.
- The reproduce worker is started once at application startup (`Engine.StartReproduceWorker`) and runs for the lifetime of the process.
- A finding grouping goroutine runs every 15s in the background, scanning for ungrouped confirmed findings and assigning group IDs. It is started alongside the reproduce worker.
- Shutdown order is strict: API → Engine (campaigns) → Proxy → DB. The grouping goroutine stops when the shared context is cancelled.

## Anti-patterns

- **DO NOT** add direct data flow from the MITM proxy to the fuzzing engine. All data must go through the database.
- **DO NOT** create task generators inside workers. The generator is a single goroutine that feeds all workers.
- **DO NOT** change the shutdown order without updating `internal/cli/serve.go`. The order prevents race conditions (API stops accepting campaign starts before engine shuts down campaigns).
- **DO NOT** make seed loading asynchronous during campaign start. The synchronous design ensures we can fail fast and report the error to the API caller.

## Related

- [`layers.md`](layers.md) — layer architecture and import rules
- [`security-model.md`](security-model.md) — TLS interception details
- [`domains/traffic-capture/README.md`](../domains/traffic-capture/README.md) — proxy and recorder domain
- [`domains/fuzzing-engine/README.md`](../domains/fuzzing-engine/README.md) — engine, mutate, replay domain
- [`domains/fuzzing-engine/README.md`](../domains/fuzzing-engine/README.md) — engine, mutate, replay domain
