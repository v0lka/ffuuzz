package engine

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"

	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/model"
)

// ucbConstant is the exploration constant of the UCB1 score formula:
//
//	score(k) = weights[k] * ( mean(k) + C * sqrt( 2 * ln(N) / n_k ) )
//
// 1.0 is a standard choice and is intentionally not configurable.
const ucbConstant = 1.0

// ExchangeRef points to an exchange inside a corpus of recording sessions:
// SessionIdx is the index in the seeds slice, ExchangeIdx is the index in
// that session's Entries slice.
type ExchangeRef struct {
	SessionIdx  int
	ExchangeIdx int
}

// epStats holds running totals for a single endpoint.
type epStats struct {
	n         int64
	rewardSum float64
	findings  int64
}

// EndpointProgress is a read-only snapshot of one endpoint's planner state.
// It is used by metrics/SSE consumers; equal-named fields stay in sync with
// the worker-side reward attribution rules.
type EndpointProgress struct {
	Key        endpoint.Key
	Tests      int64
	Findings   int64
	MeanReward float64
}

// EndpointPlanner schedules per-endpoint targeted fuzzing tasks across the
// recording corpus using UCB1 plus an optional soft floor.
//
// The planner is initialised from a slice of seed sessions; every
// (sessionIdx, exchangeIdx) pair is collapsed onto an endpoint.Key derived
// via endpoint.KeyFromExchange. Disabled overrides remove the key from the
// candidate set; weight overrides scale the UCB score. New keys (n=0) win
// the initial picks until every active key has at least one observation.
//
// EndpointPlanner is safe for concurrent use; all public methods take an
// internal mutex.
type EndpointPlanner struct {
	mu       sync.Mutex
	keys     []endpoint.Key
	stats    map[endpoint.Key]*epStats
	sessions map[endpoint.Key][]ExchangeRef
	weights  map[endpoint.Key]float64
	floor    int
	totalN   int64
	lastKey  endpoint.Key
	rng      *rand.Rand
}

// NewEndpointPlanner constructs a planner for the given seeds. Overrides may
// disable specific endpoints or scale their scheduling weights. minFloor sets
// the soft minimum number of tests per endpoint before UCB1 takes over (0
// disables the floor).
//
// Returns an error if no enabled endpoints remain after applying overrides.
func NewEndpointPlanner(
	seeds []model.RecordingSession,
	overrides []model.EndpointWeightOverride,
	minFloor int,
	rngSeed int64,
) (*EndpointPlanner, error) {
	if minFloor < 0 {
		return nil, fmt.Errorf("minFloor must be >= 0, got %d", minFloor)
	}

	sessions := make(map[endpoint.Key][]ExchangeRef)
	for sIdx, sess := range seeds {
		for eIdx, ex := range sess.Entries {
			k := endpoint.KeyFromExchange(ex)
			sessions[k] = append(sessions[k], ExchangeRef{SessionIdx: sIdx, ExchangeIdx: eIdx})
		}
	}

	disabled := make(map[endpoint.Key]bool)
	weights := make(map[endpoint.Key]float64)
	for _, ov := range overrides {
		applyOverride(sessions, ov, disabled, weights)
	}

	keys := make([]endpoint.Key, 0, len(sessions))
	stats := make(map[endpoint.Key]*epStats, len(sessions))
	for k := range sessions {
		if disabled[k] {
			continue
		}
		keys = append(keys, k)
		stats[k] = &epStats{}
		if _, ok := weights[k]; !ok {
			weights[k] = 1.0
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("no enabled endpoints to fuzz")
	}

	// Stable order keeps deterministic UCB tie-breaking under a fixed rng seed.
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Method != keys[j].Method {
			return keys[i].Method < keys[j].Method
		}
		return keys[i].Path < keys[j].Path
	})

	return &EndpointPlanner{
		keys:     keys,
		stats:    stats,
		sessions: sessions,
		weights:  weights,
		floor:    minFloor,
		rng:      rand.New(rand.NewSource(rngSeed)),
	}, nil
}

// applyOverride matches a single override against the seed-derived sessions
// map and updates disabled/weights accordingly. An empty Method matches
// every method on the given Path.
func applyOverride(
	sessions map[endpoint.Key][]ExchangeRef,
	ov model.EndpointWeightOverride,
	disabled map[endpoint.Key]bool,
	weights map[endpoint.Key]float64,
) {
	for k := range sessions {
		if k.Path != ov.Path {
			continue
		}
		if ov.Method != "" && k.Method != ov.Method {
			continue
		}
		if ov.Disabled {
			disabled[k] = true
			continue
		}
		if ov.Weight > 0 {
			weights[k] = ov.Weight
		}
	}
}

// Pick chooses the next endpoint to fuzz and one of its concrete exchanges.
// Floor phase: while any active key has fewer than floor observations, pick
// uniformly among under-floor keys. Otherwise apply UCB1 weighted by user
// weights; new keys (n=0) win immediately. The chosen exchange is sampled
// uniformly from the key's exchange refs.
//
// Pick advances internal counters, including the planner's shared totalN.
// The returned ExchangeRef indexes into the original seeds slice supplied to
// NewEndpointPlanner.
func (p *EndpointPlanner) Pick() (endpoint.Key, ExchangeRef) {
	p.mu.Lock()
	defer p.mu.Unlock()

	var chosen endpoint.Key
	if p.floor > 0 {
		under := make([]endpoint.Key, 0, len(p.keys))
		for _, k := range p.keys {
			if p.stats[k].n < int64(p.floor) {
				under = append(under, k)
			}
		}
		if len(under) > 0 {
			chosen = under[p.rng.Intn(len(under))]
		}
	}

	if chosen == (endpoint.Key{}) {
		chosen = p.argmaxUCB()
	}

	refs := p.sessions[chosen]
	ref := refs[p.rng.Intn(len(refs))]

	p.stats[chosen].n++
	p.totalN++
	p.lastKey = chosen
	return chosen, ref
}

// argmaxUCB returns the active key with the highest UCB1 score. Caller must
// hold p.mu. New keys (n=0) yield score=+Inf and win the tie. Among equal
// scores the lexicographically first key wins (keys are kept sorted).
func (p *EndpointPlanner) argmaxUCB() endpoint.Key {
	totalN := p.totalN
	if totalN < 1 {
		totalN = 1
	}
	logN := math.Log(float64(totalN))

	var best endpoint.Key
	bestScore := math.Inf(-1)
	for _, k := range p.keys {
		s := p.stats[k]
		w := p.weights[k]
		var score float64
		if s.n == 0 {
			score = math.Inf(1)
		} else {
			mean := s.rewardSum / float64(s.n)
			explore := ucbConstant * math.Sqrt(2*logN/float64(s.n))
			score = w * (mean + explore)
		}
		if score > bestScore {
			bestScore = score
			best = k
		}
	}
	return best
}

// Reward credits an endpoint with the given reward delta. Negative deltas
// are accepted but unusual; callers normally pass non-negative scores.
func (p *EndpointPlanner) Reward(k endpoint.Key, delta float64) {
	if delta == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.stats[k]; ok {
		s.rewardSum += delta
	}
}

// RecordFinding increments the finding counter for k and adds a fixed
// finding reward (kept in sync with the worker reward coefficients used by
// SeedInterestTracker: novel status +2, novel error +3, finding +5).
func (p *EndpointPlanner) RecordFinding(k endpoint.Key) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s, ok := p.stats[k]; ok {
		s.findings++
		s.rewardSum += 5.0
	}
}

// LastPickedKey returns the most recent key returned by Pick. Useful when
// the caller wants to attach the key to a SeedTask without re-deriving it.
func (p *EndpointPlanner) LastPickedKey() endpoint.Key {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastKey
}

// Snapshot returns a stable copy of per-endpoint counters. The returned
// slice is sorted by key for deterministic output.
func (p *EndpointPlanner) Snapshot() []EndpointProgress {
	p.mu.Lock()
	defer p.mu.Unlock()

	out := make([]EndpointProgress, 0, len(p.keys))
	for _, k := range p.keys {
		s := p.stats[k]
		var mean float64
		if s.n > 0 {
			mean = s.rewardSum / float64(s.n)
		}
		out = append(out, EndpointProgress{
			Key:        k,
			Tests:      s.n,
			Findings:   s.findings,
			MeanReward: mean,
		})
	}
	return out
}
