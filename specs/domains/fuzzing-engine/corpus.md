# Corpus

## Responsibility

Loads recording sessions (seeds) for fuzzing campaigns and computes per-endpoint P50 latency baselines from the recorded traffic. The corpus provides the input seeds and baseline data that the fuzzing engine uses to drive mutation and anomaly detection.

## Key Types

```go
type Manager struct {
    recordings RecordingReader
    campaigns  CampaignReader
    logger     zerolog.Logger
}

type RecordingReader interface {
    // Reads recording sessions used as seeds for a campaign.
}

type CampaignReader interface {
    // Reads campaign metadata to determine which recordings to use.
}

type BaselineEntry struct {
    Method     string
    Endpoint   string
    P50Ms      int64
    StatusCode int
}
```

## Public API

### `NewManager(recordings RecordingReader, campaigns CampaignReader, logger zerolog.Logger) *Manager`
Creates a corpus manager. Both `RecordingReader` and `CampaignReader` are interfaces defined in the corpus package (not exported from `db`).

### `GetSeeds(ctx context.Context, campaignID string) ([]model.RecordingSession, error)`
Loads all recording sessions associated with the campaign. Reads the campaign's `RecordingIDs` list via `CampaignReader`, then fetches each recording with its exchanges via `RecordingReader`. Returns the full sessions ready for mutation.

### `ComputeBaseline(sessions []model.RecordingSession) map[string]model.BaselineEntry`
Standalone function (no receiver). Computes per-endpoint P50 latency baselines from recorded exchanges:

1. Groups all exchanges by `METHOD + endpoint` key
2. For each group, sorts durations and finds the median (P50)
3. Stores the baseline status code (from the first exchange at or near P50)
4. Returns `map[string]model.BaselineEntry` keyed by `"METHOD|endpoint"`

## Baseline Computation

```
Input: []RecordingSession (each with []Exchange, each with DurationMs)

1. Flatten: collect all exchanges from all sessions
2. Group by key: "METHOD|endpoint" (e.g., "GET|/api/users/{_}")
3. For each group:
   a. Sort by DurationMs
   b. P50Ms = exchanges[len(exchanges)/2].DurationMs
   c. StatusCode = status from the exchange at or near P50 index
4. Return map[key]BaselineEntry{METHOD, endpoint, P50Ms, StatusCode}
```

The baseline `StatusCode` is used by `ServerErrorDetector` to skip flagging endpoints that normally return 5xx responses.

## Flow in Engine

```
StartCampaign:
    │
    ├── corpus.GetSeeds(campaignID)
    │   └── Returns []RecordingSession (seeds with exchanges)
    │
    ├── corpus.ComputeBaseline(seeds)
    │   └── Returns map[string]BaselineEntry (per-endpoint P50)
    │
    └── Pass seeds and baselines to runCampaign()
        │
        ├── Workers iterate seeds for mutation
        └── Detectors use baselines for anomaly comparison
```

## Invariants

- `GetSeeds` is called exactly once per campaign, during `StartCampaign`. Seeds are loaded synchronously before any worker goroutine starts.
- `ComputeBaseline` operates on the loaded seeds, not on the entire database. Baselines reflect the recorded traffic, not real-time observation.
- The group key for baselines is `"METHOD|endpoint"` where endpoint is the normalised path (e.g., `GET|/api/users/{_}`).
- `ComputeBaseline` is a pure function: it takes `[]RecordingSession` as input and returns a `map` as output. No I/O, no side effects.
- Baselines are recomputed each time a campaign starts. They are not cached between campaigns.

## Dependencies

| Package | Used for |
|---------|----------|
| `internal/model` | `RecordingSession`, `Exchange`, `BaselineEntry`, `Campaign` |

## Edge Cases

- **No seeds available**: `GetSeeds` returns empty slice. Caller (Engine) transitions campaign to FAILED.
- **Campaign with no recordings assigned**: Same as no seeds.
- **All exchanges have identical durations**: P50 is the same as every value. Baseline sets P50 equal to the uniform duration.
- **Single exchange per endpoint**: P50 = that exchange's duration (no sorting ambiguity).
- **Mixed status codes**: Baseline `StatusCode` is from the exchange at the P50 index, not the most common code. This is acceptable for the detector's "skip if baseline was 5xx" logic.
