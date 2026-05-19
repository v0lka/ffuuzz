# CLI → Infrastructure

## Overview

The CLI package (`internal/cli/`) is the composition root that wires all infrastructure components together at startup. It depends on every infrastructure package and creates the concrete implementations that boundary packages consume.

## Interfaces

The CLI does not define its own interfaces — it instantiates concrete types directly:

```go
// cli/serve.go

// 1. Config
cfg, err := config.Load(args)

// 2. Database
database, err := db.Open(cfg.DatabaseURI, logger)

// 3. Stores (db layer)
recordingStore := db.NewRecordingStore(database.DB, logger)
campaignStore  := db.NewCampaignStore(database.DB, logger)
findingStore   := db.NewFindingStore(database.DB, logger)
artifactStore  := db.NewArtifactStore(database.DB, logger)

// 4. Corpus Manager
corpusMgr := corpus.NewManager(recordingStore, campaignStore, logger)

// 5. LLM Provider (graceful degradation when disabled)
llmProvider, err := llm.NewProvider(cfg.LLM, logger)
var llmTriager *triage.LLMTriager
if llmProvider != nil {
    llmTriager = triage.NewLLMTriager(llmProvider, logger)
}

// 6. Engine
eng := engine.NewEngine(campaignStore, findingStore, artifactStore, corpusMgr, llmTriager, cfg.ArtifactDir, logger)

// 7. Endpoint Resolver
resolver := endpoint.NewResolver(recordingStore, logger)
resolver.RebuildFromDB(context.Background())

// 8. Recorder
rec := recorder.NewDBRecorder(recordingStore, resolver, logger)

// 9. Cert Store
cs, err := store.NewCertStore(cfg.CertCache, cfg.TLS, logger)

// 10. MITM Proxy
proxy := mitm.New(mitm.Config{
    ListenAddr:    cfg.ProxyAddress,
    CertStore:     cs,
    Recorder:      rec,
    MaxBodyBytes:  cfg.MaxBodyBytes,
    TLSSkipVerify: cfg.TLSSkipVerify,
    Logger:        logger,
})

// 11. API Server
apiSrv := api.NewServer(api.ServerConfig{
    Addr:        cfg.APIAddress,
    Recordings:  recordingStore,
    Campaigns:   campaignStore,
    Findings:    findingStore,
    Artifacts:   artifactStore,
    Engine:      eng,
    Health:      database,
    LLMTriager:  llmTriager,
    ArtifactDir: cfg.ArtifactDir,
    WebFS:       webFS,
    Logger:      logger,
})

// 12. Vulnerability grouping (background goroutine)
go runFindingGroupingLoop(ctx, findingStore, triage.NewTriager(), 15*time.Second, logger)
```

## Initialization

The wiring happens in `internal/cli/serve.go:runServe()` in the `serve` subcommand. The order is determined by dependency chains:

1. **Config** — must load first, everything depends on it. Loads `.env` file (optional, supports `${VAR}` expansion), then environment variables (`FFUUZZ_*`), then CLI flags in priority order: CLI > env > `.env` > defaults.
2. **Database** — must open before any store is created
3. **Stores** — depend on `*sqlx.DB`
4. **Corpus Manager** — depends on `RecordingStore` + `CampaignStore`
5. **LLM Provider** — created from config (`FFUUZZ_LLM_*`); graceful degradation when `LLM_ENABLED=false`
6. **Engine** — depends on all stores + corpus manager + LLM triager
7. **Endpoint Resolver** — depends on `RecordingStore` (via `Merger` interface implementation)
8. **Recorder** — depends on `RecordingStore` + `Resolver`
9. **Cert Store** — depends on `CertCacheConfig` + `TLSConfig`
10. **MITM Proxy** — depends on `CertStore` + `Recorder`
11. **API Server** — depends on all stores + `Engine` + `LLMTriager` + WebFS
12. **Vulnerability Grouping Loop** — depends on `FindingStore`; periodically groups ungrouped confirmed findings (every 15s)

## Data Flow

```
config.Load(args) ──────────► cfg
                                  │
db.Open(dsn, logger) ─────────► database
                                  │
                ┌─────────────────┼──────────────────┐
                ▼                 ▼                   ▼
        recordingStore    campaignStore    findingStore   artifactStore
                │                 │                   │           │
        ┌───────┼─────────┐       │                   │           │
        ▼       ▼         ▼       ▼                   ▼           │
    resolver  corpus   recorder  engine ◄─────────────┘           │
        │       │         │       │    llmProvider                │
        │       │         │       │    llmTriager                 │
        │       │         │       │       │                       │
        ▼       │         ▼       │       │                       │
      (trie)    │     proxy       │       │                       │
                │                 │       │                       │
                └─────────────────┴───────┴───────────────────────┘
                                  │
                                  ▼
                            api.NewServer
                                  │
                    ┌─────────────┼─────────────┐
                    ▼             ▼             ▼
              ListenAndServe  ListenAndServe  StartReproduceWorker
              (API :8081)     (Proxy :8080)   (background)
                                              runFindingGroupingLoop
                                              (periodic, 15s)
```

## Shutdown

```
SIGINT/SIGTERM
    │
    ├── 1. apiSrv.Shutdown(ctx)       // stop accepting new API requests
    ├── 2. eng.StopAll(ctx)           // stop reproduce worker + all campaigns
    ├── 3. proxy.Shutdown(ctx)        // stop MITM proxy
    └── 4. database.Close()           // close PostgreSQL connection (deferred)
```

## Breaking Change Checklist

- [ ] Does the new component depend on a store that hasn't been created yet?
- [ ] Is the initialization order preserved (config → db → stores → engine → proxy → api)?
- [ ] Is the shutdown order still correct (api → engine → proxy → db)?
- [ ] Are all `Fatal` errors appropriate? (config load, DB connection, cert store creation are fatal; others are recoverable)
- [ ] Does the new component need graceful shutdown support?

## Related

- [`api-engine.md`](api-engine.md) — API → Engine boundary
- [`api-db.md`](api-db.md) — API → DB boundary
- [`proxy-recorder.md`](proxy-recorder.md) — MITM → Recorder boundary
