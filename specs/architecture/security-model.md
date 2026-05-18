# Security Model

## Overview

FFUUZZ operates as a local security testing tool that intercepts HTTPS traffic through MITM techniques. It generates its own root CA certificate, dynamically creates per-hostname leaf certificates, and uses them to decrypt TLS connections for analysis. The security model prioritises isolation: certificates are local-scope only, TLS verification against upstream servers can be relaxed for testing, and variable extraction from responses carries defined safety boundaries.

## Diagram

```
                        TLS INTERCEPTION FLOW
   ┌──────────┐                              ┌──────────┐
   │  Client  │── CONNECT target.com:443 ──► │  Proxy   │
   │(browser) │                              │  (:8080) │
   └──────────┘                              └────┬─────┘
                                                  │
                   ┌──────────────────────────────┤
                   │                              │
                   ▼                              ▼
   ┌──────────────────────────┐    ┌──────────────────────────┐
   │  CertStore               │    │  TLS to Origin            │
   │                          │    │                          │
   │  1. Load/generate root CA│    │  client.DialTLS("tcp",   │
   │  2. GetCertFor(host)     │    │    "target.com:443",     │
   │  3. LRU cache lookup     │    │    tls.Config{           │
   │  4. if miss: sign leaf   │    │      InsecureSkipVerify: │
   │     cert with root CA    │    │        TLSSkipVerify,    │
   │  5. persist to disk      │    │      ServerName: host,   │
   │  6. singleflight dedup   │    │    })                    │
   └──────────┬───────────────┘    └──────────────────────────┘
              │
              ▼
   ┌──────────────────────────┐
   │  MITM TLS Handshake      │
   │                          │
   │  tls.Config{             │
   │    Certificates: [leaf], │
   │    MinVersion: config,   │
   │    CipherSuites: config, │
   │    SessionTicketsDisabled│
   │  }                       │
   └──────────┬───────────────┘
              │
              ▼
         Client ←TLS→ Proxy ←TLS→ Origin
         (leaf cert)   (origin cert or skip verify)
```

## Components

### Root CA and Certificate Generation

The `store.CertStore` (`internal/store/store.go`) manages all TLS certificates:

- **Root CA**: Generated on first run and persisted to the `cert_dir` (default: `certs/`). Falls back to loading from disk on subsequent runs. The root CA is a 2048-bit RSA key with a self-signed X.509 certificate valid for 10 years.
- **Leaf certificates**: Generated on-demand per hostname via `GetCertFor(host)`. Each leaf cert is signed by the root CA, valid for 1 year, with the hostname in the Subject Common Name and DNS SAN.
- **LRU cache**: In-memory cache of generated certificates (default capacity: 1000). Cache misses trigger `singleflight.Group.Do` to prevent duplicate concurrent certificate generation for the same hostname.
- **Memory-only mode**: When `-cert-memory-only` is set, certificates are generated in-memory only and never persisted to disk.
- **Prometheus metrics** track cache hits, misses, and evictions (`CertCacheHits`, `CertCacheMisses`, `CertCacheEvictions`).

### TLS Config Hardening

The `config.TLSConfig` struct controls TLS security parameters:

```
MinVersion:        TLS 1.2 or 1.3 (default: no restriction)
HandshakeTimeout:  10s
CipherSuites:      configurable list (default: modern suites)
SessionTickets:    disabled by default (-tls-no-tickets)
```

### Upstream TLS Verification

The MITM proxy forwards traffic to the origin server using `http.Transport` with a custom `TLSClientConfig`:

- `InsecureSkipVerify`: controlled by `TLSSkipVerify` config (default: `true`). When `true`, the proxy does not verify the origin server's TLS certificate. This is typical for testing environments with self-signed certs.
- `ServerName`: set to the target hostname for SNI.

### Variable Extraction Safety

The `replayer.WorkerContext` (`internal/replayer/`) supports extracting variables from HTTP responses using regex capture groups and substituting them into subsequent requests via `{{var}}` placeholders:

- **Source**: body (regex match on response body) or header (regex match on header value)
- **Scope**: per-worker — variables persist within a single worker's `WorkerContext` and are not shared across workers
- **Defined per campaign** via `CampaignConfig.ExtractionRules`
- **No default extraction rules**: the user must explicitly configure rules. No automatic extraction occurs.

### Recorded Data Sensitivity

FFUUZZ records full HTTP request and response bodies (Base64-encoded), headers, and URLs. These are stored in PostgreSQL and potentially in artifact files on disk. The tool does not implement encryption-at-rest or data scrubbing — it is designed for local development and testing environments.

## Invariants

- The root CA private key is generated once per installation and never leaves the `cert_dir`. No key material is transmitted over the network.
- Each hostname gets exactly one leaf certificate (enforced by `singleflight`). Two concurrent requests for the same hostname will produce one certificate, not two.
- `TLSSkipVerify` defaults to `true`. This is intentional for testing: the proxy must work against targets with self-signed or invalid certificates.
- Variable extraction is opt-in. Without `CampaignConfig.ExtractionRules`, no variables are extracted or substituted.
- Extracted variables are scoped to a single worker. Worker A cannot access variables extracted by Worker B.
- Request and response bodies are truncated at `MaxBodyBytes` (default 64KB) during recording. Bodies larger than this limit are recorded as truncated.
- The API server exposes no authentication. All endpoints at `:8081` are intended for local access only.

## Anti-patterns

- **DO NOT** expose the proxy or API server to untrusted networks. The tool has no authentication and the proxy decrypts all traffic.
- **DO NOT** decrease `TLSSkipVerify` to `false` in a setting where upstream certificates are invalid — the proxy will fail to connect.
- **DO NOT** store the root CA private key outside the `cert_dir`. The key allows impersonation of any hostname.
- **DO NOT** add automatic variable extraction rules. All extraction must be explicitly configured by the user per campaign.
- **DO NOT** share `WorkerContext` across workers. Each worker must have its own state (cookies, variables, extraction rules).

## Related

- [`layers.md`](layers.md) — where cert store fits in the layer architecture
- [`data-flow.md`](data-flow.md) — TLS interception in the proxy flow
- [`domains/fuzzing-engine/replayer.md`](../domains/fuzzing-engine/replayer.md) — WorkerContext and variable substitution details
- [`contracts/cli-infrastructure.md`](../contracts/cli-infrastructure.md) — cert store wiring at startup
