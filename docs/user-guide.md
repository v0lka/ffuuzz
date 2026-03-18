# FFUZZ User Guide

FFUZZ is a web application security testing tool that combines MITM proxy traffic recording with intelligent mutation-based fuzzing. It captures HTTP/HTTPS traffic, replays it with various mutations, and detects anomalies that may indicate security vulnerabilities.

## Table of Contents

- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Commands](#commands)
- [Usage Workflow](#usage-workflow)
- [Mutation Operators](#mutation-operators)
- [Anomaly Detection](#anomaly-detection)
- [REST API Reference](#rest-api-reference)
- [Web Dashboard](#web-dashboard)
- [Testing Target (GoVWA)](#testing-target-govwa)

## Features

- **MITM Proxy** -- Intercept and decrypt HTTPS traffic with automatic certificate management
- **Traffic Recording** -- Capture complete request/response exchanges for later analysis
- **Mutation Testing** -- Apply intelligent mutations to headers, query parameters, JSON bodies, and URL paths
- **Anomaly Detection** -- Identify server errors, timeouts, latency regressions, and regex pattern matches
- **Campaign Management** -- Organize fuzzing campaigns with configurable limits and strategies
- **Finding Triage** -- Automatically confirm, reproduce, and minimize findings
- **Web Dashboard** -- React-based UI for managing recordings, campaigns, and findings
- **Diff Analysis** -- Compare responses to identify behavioral changes
- **Real-time Streaming** -- Server-Sent Events (SSE) for live campaign statistics
- **Prometheus Metrics** -- Export operational metrics at `/metrics`

## Prerequisites

- Go 1.25+
- Node.js 20+ (for building the web UI from source)
- PostgreSQL 16+ (or use Docker Compose)

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd ffuuzz

# Start PostgreSQL
docker-compose up -d postgres

# Build the application (frontend + backend)
make build

# Run the server
./ffuuzz serve
```

The application will be available at:

| Service   | Address                |
|-----------|------------------------|
| Proxy     | `http://localhost:8080` |
| Web UI    | `http://localhost:8081` |
| Health    | `http://localhost:8081/healthz` |
| Metrics   | `http://localhost:8081/metrics` |

## Configuration

Configuration is loaded from environment variables first, then overridden by CLI flags.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FFUUZZ_API_ADDRESS` | `:8081` | Control API listen address |
| `FFUUZZ_PROXY_ADDRESS` | `:8080` | MITM proxy listen address |
| `FFUUZZ_DATABASE_URI` | `postgres://ffuuzz:ffuuzz@localhost:5432/ffuuzz?sslmode=disable` | PostgreSQL connection URI |
| `FFUUZZ_ARTIFACT_DIR` | `./artifacts` | Artifact storage directory |
| `FFUUZZ_WORKERS` | `8` | Number of fuzzing workers |
| `FFUUZZ_RPS` | `50` | Requests per second limit |
| `FFUUZZ_REQ_TIMEOUT` | `3s` | Per-request timeout (Go duration) |
| `FFUUZZ_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `FFUUZZ_TLS_SKIP_VERIFY` | `true` | Skip TLS verification for upstream |

### CLI Flags (`serve` command)

| Flag | Default | Description |
|------|---------|-------------|
| `-a` | `:8081` | Control API listen address |
| `-p` | `:8080` | MITM proxy listen address |
| `-d` | (see env) | PostgreSQL connection URI |
| `-o` | `./artifacts` | Artifact storage directory |
| `-cert-dir` | `certs` | Certificate directory for CA and leaf certs |
| `-max-body` | `65536` | Max body bytes to record |
| `-cert-cache-size` | `1000` | Certificate LRU cache max entries |
| `-cert-memory-only` | `false` | Keep certs in memory only (no disk) |
| `-tls-no-tickets` | `false` | Disable TLS session tickets |
| `-tls-skip-verify` | `true` | Skip TLS certificate verification for upstream |

## Commands

### `ffuuzz serve`

Run the full application: MITM proxy + Control API + fuzzing engine.

```bash
ffuuzz serve [flags]
```

This is the primary production command. It starts all components, connects to PostgreSQL, and serves the embedded web UI.

### `ffuuzz proxy`

Run the MITM proxy in standalone development mode. Traffic is recorded to a JSONL file instead of the database.

```bash
ffuuzz proxy -port 8080 -out log.jsonl
```

| Flag | Default | Description |
|------|---------|-------------|
| `-port` | `8080` | Port to listen on |
| `-out` | `log.jsonl` | JSONL output file |
| `-cert-dir` | `certs` | Certificate directory |
| `-maxbodykb` | `64` | Max body KB to record |

### `ffuuzz record`

Analyze a previously recorded JSONL log file and print a JSON summary to stdout.

```bash
ffuuzz record -in log.jsonl
```

| Flag | Default | Description |
|------|---------|-------------|
| `-in` | `log.jsonl` | JSONL input file |

## Usage Workflow

### 1. Capture Traffic

Configure your browser or application to use the MITM proxy at `localhost:8080`.

For HTTPS interception, install the CA certificate into your system or browser trust store. The CA certificate is generated at `certs/ca.pem` on first run.

```
# macOS: add to system keychain
sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain certs/ca.pem

# Linux: copy to trusted certificates
sudo cp certs/ca.pem /usr/local/share/ca-certificates/ffuuzz-ca.crt
sudo update-ca-certificates
```

Browse the target application normally -- FFUZZ will capture all traffic.

### 2. Create a Fuzzing Campaign

Use the Web UI at `http://localhost:8081` or the REST API:

```bash
curl -X POST http://localhost:8081/api/v1/campaigns \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "My Campaign",
    "recording_ids": ["<recording-id>"],
    "config": {
      "target": { "base_url": "http://target:8080" },
      "limits": {
        "workers": 4,
        "rps": 20,
        "max_tests": 1000,
        "duration_sec": 300,
        "req_timeout_ms": 3000
      },
      "mutations": {
        "path_query": true,
        "headers": true,
        "json_body": true,
        "params": true,
        "sequence": false,
        "intensity": 0.5
      },
      "anomaly": {
        "detect_5xx": true,
        "latency_multiplier": 3.0,
        "regex_patterns": ["error", "exception", "stack trace"]
      },
      "triage": {
        "confirm_runs": 3,
        "enable_minimization": true
      }
    }
  }'
```

**Campaign configuration fields:**

| Section | Field | Description |
|---------|-------|-------------|
| `target` | `base_url` | Base URL of the target application. Auto-derived from the first recording if omitted. |
| `limits` | `workers` | Number of concurrent fuzzing workers (must be > 0) |
| `limits` | `rps` | Maximum requests per second (must be > 0) |
| `limits` | `max_tests` | Maximum number of test cases to run |
| `limits` | `duration_sec` | Maximum campaign duration in seconds |
| `limits` | `req_timeout_ms` | Per-request timeout in milliseconds (must be > 0) |
| `mutations` | `path_query` | Enable path/query string mutations |
| `mutations` | `headers` | Enable header mutations |
| `mutations` | `json_body` | Enable JSON body mutations |
| `mutations` | `params` | Enable parameter mutations |
| `mutations` | `sequence` | Enable multi-exchange sequence mutations |
| `mutations` | `intensity` | Mutation intensity from 0.0 (minimal) to 1.0 (aggressive) |
| `anomaly` | `detect_5xx` | Detect HTTP 5xx server errors |
| `anomaly` | `latency_multiplier` | Flag responses slower than baseline * multiplier |
| `anomaly` | `regex_patterns` | Regex patterns to match against response bodies |
| `triage` | `confirm_runs` | Number of re-runs to confirm a finding |
| `triage` | `enable_minimization` | Attempt to minimize mutation payload |

At least one of `limits.duration_sec` or `limits.max_tests` must be greater than 0.

### 3. Start the Campaign

```bash
curl -X POST http://localhost:8081/api/v1/campaigns/<id>/start
```

The engine will:
- Load recording sessions as seeds
- Replay each exchange with mutations applied
- Detect anomalies based on the configured rules
- Save findings with full request/response artifacts
- Track real-time statistics

### 4. Monitor Progress

Stream real-time statistics via SSE:

```bash
curl -N http://localhost:8081/api/v1/campaigns/<id>/stream
```

Or poll the stats endpoint:

```bash
curl http://localhost:8081/api/v1/campaigns/<id>/stats
```

### 5. Stop the Campaign

```bash
curl -X POST http://localhost:8081/api/v1/campaigns/<id>/stop
```

Campaigns also stop automatically when `max_tests` or `duration_sec` limits are reached.

### 6. Analyze Findings

List findings for a campaign:

```bash
curl http://localhost:8081/api/v1/campaigns/<id>/findings
```

Filter by type and status:

```bash
curl 'http://localhost:8081/api/v1/findings?campaign_id=<id>&type=SERVER_ERROR&status=CONFIRMED'
```

Download the reproduction artifact:

```bash
curl http://localhost:8081/api/v1/findings/<finding-id>/artifact
```

Trigger reproduction to confirm a finding:

```bash
curl -X POST http://localhost:8081/api/v1/findings/<finding-id>/reproduce \
  -H 'Content-Type: application/json' \
  -d '{"runs": 5}'
```

## Mutation Operators

FFUZZ applies the following mutation strategies to recorded HTTP exchanges:

### Path/Query Mutations
Inject security payloads into URL paths and query strings:
- SQL injection (`' OR '1'='1`)
- Path traversal (`../../../etc/passwd`)
- Command injection
- SSTI (`{{7*7}}`)
- Log4Shell (`${jndi:ldap://...}`)

### Header Mutations
Mutate HTTP headers:
- `Content-Type` confusion
- `Authorization` tampering
- Custom header injection
- CRLF injection (`\r\nX-Injected: true`)

### JSON Body Mutations
Apply mutations to JSON request bodies:
- Type confusion (string to number, null, boolean)
- Boundary values (empty strings, extremely long strings)
- Format string injections
- Unicode edge cases (`\u0000`, `\uFFFD`)

### Parameter Mutations
Mutate URL query parameters:
- Value replacement with fuzz strings
- Parameter pollution
- Type confusion

### Sequence Mutations
Mutate multi-exchange sequences:
- Request reordering
- Request duplication
- Request omission

### Primitive Mutations
Low-level byte mutations:
- Bit flips
- Truncation
- Boundary values

**Mutation intensity** (0.0 - 1.0) controls how aggressively mutations are applied. Lower values produce minimal changes; higher values stack multiple mutations per request.

## Anomaly Detection

Findings are categorized into the following types:

| Type | Description |
|------|-------------|
| `TIMEOUT` | Request exceeded the configured timeout (`req_timeout_ms`) |
| `SERVER_ERROR` | HTTP 5xx response received from the target |
| `LATENCY_REGRESSION` | Response time significantly higher than the baseline (controlled by `latency_multiplier`) |
| `REGEX_MATCH` | Response body matches one of the configured `regex_patterns` |

### Finding Lifecycle

1. **UNCONFIRMED** -- Initially detected anomaly
2. **Reproduce** -- The triage system re-runs the request `confirm_runs` times
3. **CONFIRMED** -- Anomaly reproduced consistently
4. **Minimization** -- If enabled, the engine attempts to reduce the mutation to the minimal reproducing payload

### Finding Details

Each finding includes:
- The HTTP method and endpoint
- Mutation type and payload used
- Baseline vs. observed latency (for latency regressions)
- HTTP status code (for server errors)
- Full request/response artifact for reproduction
- Seed recording ID for traceability

## REST API Reference

Base URL: `http://localhost:8081/api/v1`

All endpoints return JSON. Errors follow the format:

```json
{
  "error": "ERROR_CODE",
  "message": "Human-readable description",
  "request_id": "uuid"
}
```

### Health & Metrics

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Health check (returns DB status, version) |
| GET | `/metrics` | Prometheus metrics |

### Recordings

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/recordings/import` | Import recording sessions (JSON body) |
| GET | `/api/v1/recordings` | List recordings. Query: `limit`, `offset`, `host`, `path_prefix` |
| GET | `/api/v1/recordings/tree` | Get recordings as a hierarchical tree by origin/path |
| GET | `/api/v1/recordings/export` | Export all recordings as JSON download. Query: `host`, `path_prefix` |
| GET | `/api/v1/recordings/:id` | Get a single recording. Query: `include_entries=true`, `max_body_bytes` |
| DELETE | `/api/v1/recordings/:id` | Delete a recording (fails if used by an active campaign) |
| DELETE | `/api/v1/recordings/by-prefix` | Bulk delete by origin. Query: `scheme`, `host`, `port`, `path_prefix` |

### Campaigns

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/campaigns` | Create a new campaign |
| GET | `/api/v1/campaigns` | List campaigns. Query: `status`, `limit`, `offset` |
| GET | `/api/v1/campaigns/:id` | Get campaign details |
| GET | `/api/v1/campaigns/:id/stats` | Get aggregated campaign statistics |
| GET | `/api/v1/campaigns/:id/config` | Get campaign configuration |
| GET | `/api/v1/campaigns/:id/findings` | List findings for a campaign. Query: `type`, `status`, `since`, `limit`, `offset` |
| GET | `/api/v1/campaigns/:id/stream` | SSE stream of real-time campaign statistics |
| POST | `/api/v1/campaigns/:id/start` | Start a campaign |
| POST | `/api/v1/campaigns/:id/stop` | Stop a running campaign |
| POST | `/api/v1/campaigns/:id/recordings` | Add recordings to a campaign by filter |

**Campaign statuses:** `CREATED`, `STARTING`, `RUNNING`, `STOPPING`, `STOPPED`, `FINISHED`, `FAILED`

### Findings

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/findings` | List all findings. Query: `campaign_id`, `type`, `status`, `since`, `limit`, `offset` |
| GET | `/api/v1/findings/:id` | Get a single finding |
| GET | `/api/v1/findings/:id/artifact` | Download finding reproduction artifact (JSON) |
| POST | `/api/v1/findings/:id/reproduce` | Enqueue finding for reproduction. Body: `{"runs": N}` (1-20, default 3) |

### Pagination

All list endpoints support `limit` (default 50) and `offset` (default 0) query parameters.

### Filtering by Time

Endpoints that accept a `since` parameter expect an RFC 3339 timestamp:

```
?since=2025-01-15T10:30:00Z
```

## Web Dashboard

The web UI is available at `http://localhost:8081/ui/`. It provides:

- **Recordings view** -- Browse captured traffic organized by origin and path
- **Campaign management** -- Create, start, stop, and monitor fuzzing campaigns
- **Findings browser** -- Filter and inspect findings by type and status
- **Real-time stats** -- Live campaign progress via SSE
- **Artifact viewer** -- Inspect full request/response payloads for reproduction

## Testing Target (GoVWA)

For testing purposes, you can use [GoVWA](https://github.com/0c34/govwa) (Go Vulnerable Web Application), a deliberately vulnerable web application designed for learning web application security testing.

> **Warning**: GoVWA is intentionally vulnerable. Only run it in isolated local environments.

### Starting GoVWA

Add GoVWA services to your Docker Compose setup and run:

```bash
docker-compose up -d --build
```

This starts:

| Service | Port |
|---------|------|
| PostgreSQL | `5432` |
| GoVWA | `8888` |
| GoVWA MySQL | `3307` |

### GoVWA Credentials

| Username | Password |
|----------|----------|
| admin | govwaadmin |
| user1 | govwauser1 |

### Using GoVWA with FFUZZ

1. Configure your browser to use the FFUZZ proxy at `localhost:8080`
2. Navigate to `http://localhost:8888` and log in
3. Browse GoVWA features -- FFUZZ will capture all traffic
4. Create a fuzzing campaign targeting `http://localhost:8888`
