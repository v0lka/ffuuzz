# ADR-003: MITM TLS Interception with On-the-Fly Certificates

## Status
Accepted

## Context
FFUUZZ must intercept and record HTTPS traffic to capture realistic API interactions for fuzzing. To do this, it needs to decrypt TLS connections between the client and the target server. Standard approaches include: (1) requiring users to install a pre-generated CA certificate, (2) using an existing CA from the system trust store, or (3) generating a root CA on first run.

The tool also needs to handle high connection volumes during recording — potentially hundreds of CONNECT requests per second to different hostnames, each requiring a unique TLS certificate.

## Decision
Generate a root CA certificate on first run and persist it to disk (`cert_dir`). For each intercepted hostname, generate a leaf certificate signed by the root CA on demand. Cache leaf certificates in an in-memory LRU cache with `singleflight` deduplication to prevent concurrent certificate generation for the same hostname.

**Certificate configuration**:
- Root CA: RSA 2048-bit key, valid 10 years, persisted to `cert_dir`
- Leaf certificates: RSA 2048-bit key, valid 1 year, Subject CN = hostname, DNS SAN = hostname
- LRU cache: configurable size (default 1000 entries)
- `singleflight.Group.Do()` prevents duplicate generation under concurrent requests

**Memory-only mode**: When `-cert-memory-only` is set, certificates are not persisted to disk. Cache evictions permanently lose the certificate, requiring regeneration.

## Consequences

### Positive
- The root CA is generated locally — no external CA dependency or pre-shared certificate needed
- Users only need to trust one root CA in their browser/OS, not individual host certificates
- LRU cache + singleflight makes the system efficient under concurrent load
- Memory-only mode is suitable for ephemeral environments (CI, containers)
- Prometheus metrics track cache performance (hits, misses, evictions)

### Negative
- Users must manually trust the generated root CA in their browser/OS to avoid TLS warnings
- Root CA private key is stored on disk — file system permissions are the only protection
- Each new hostname incurs RSA key generation overhead (mitigated by singleflight and LRU cache)
- The root CA is valid for 10 years — if compromised, it can impersonate any hostname
- Memory-only mode requires regeneration on cache eviction if the same hostname is visited again

## Alternatives Considered

- **Use system trust store**: Would not require user to trust a custom CA. Rejected because it would require root/administrator privileges to access system keychains, and cross-platform support is complex.
- **Pre-generated wildcard certificate**: Simpler but only works for known hostnames. Rejected because the tool works with arbitrary targets.
- **mitmproxy library**: Existing Python-based MITM proxy. Rejected for Go integration — would require subprocess management or cgo.
- **No certificate caching**: Would generate a new certificate for every CONNECT request. Rejected for performance — RSA key generation is expensive under load.
