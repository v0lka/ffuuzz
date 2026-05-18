# ADR-004: Mutation-Based Fuzzing Approach

## Status
Accepted

## Context
FFUUZZ needs to generate test cases that can discover security vulnerabilities in web APIs. Common approaches include: (1) grammar-based fuzzing (generate inputs from a formal grammar), (2) mutation-based fuzzing (mutate valid inputs), (3) generation-based fuzzing (generate inputs from scratch using heuristics), or (4) symbolic execution (analyze code paths).

The tool's primary input is recorded HTTP traffic — valid, real-world API interactions captured through the MITM proxy. This provides a corpus of valid requests that can serve as mutation templates.

## Decision
Use mutation-based fuzzing where recorded HTTP exchanges are mutated using multiple strategies organized in a pipeline. Each mutation class (URI, header, JSON body, params, sequence, primitive byte-level) is applied probabilistically based on configured intensity. The pipeline guarantees at least one mutation is always applied (primitive fallback).

**Mutation strategies** are organized into independent classes that can be enabled/disabled per campaign:
- URI mutations: path segments, query params, encoding, reserved chars
- Header mutations: add/remove/duplicate/long/conflicting headers
- JSON body mutations: type substitution, key manipulation, boundaries, injection
- Parameter injection: fuzz strings in query/form params
- Sequence mutations: drop/duplicate/swap/reorder exchanges
- Primitive mutations: bitflip, byteflip, arithmetic, block ops, splice

Each mutation uses a deterministic RNG seeded from the task's mutation seed for reproducibility.

## Consequences

### Positive
- Leverages real traffic as mutation templates — mutations stay within realistic API structure
- Mutation-based approach produces fewer invalid requests than grammar-based or generation-based approaches
- Deterministic RNG enables reproducibility — a finding can be reproduced with the same mutation seed
- Pipeline architecture makes it easy to add new mutation strategies
- Per-class enable/disable + intensity control gives users fine-grained configuration
- At least one mutation is always applied (primitive fallback) — no "no-op" mutations

### Negative
- Limited to mutations of existing traffic — cannot discover vulnerabilities on unvisited API endpoints
- Mutation quality depends on seed traffic diversity — limited traffic = limited fuzzing surface
- Primitive byte-level mutations may produce invalid HTTP that the replayer rejects
- Inherits any bias in the recorded traffic (e.g., over-represented endpoints get more testing)
- No awareness of API semantics — cannot do targeted testing of specific parameter types

## Alternatives Considered

- **Grammar-based fuzzing (e.g., from OpenAPI spec)**: Would provide better structural coverage but requires an API specification (not always available) and generates synthetic requests that may not match real usage patterns.
- **Generation-based fuzzing**: Would work without recorded traffic but produces highly invalid requests and is difficult to tune for web APIs specifically.
- **Coverage-guided fuzzing (libFuzzer-style)**: Requires instrumented target binaries, which is not feasible for black-box web API testing.
- **Hybrid mutation + generation**: Would add complexity without clear benefit for the initial scope. Could be revisited if mutation-only results prove insufficient.
