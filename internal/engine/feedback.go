package engine

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// SeedInterestTracker tracks per-seed response diversity for coverage-guided
// seed selection. Seeds that produce novel status codes, error messages, or
// findings receive higher interest scores and are prioritized by the task generator.
// It is safe for concurrent use from multiple worker goroutines.
type SeedInterestTracker struct {
	mu     sync.Mutex
	scores map[string]*seedStats // seedID → stats
}

type seedStats struct {
	interest    float64
	statusCodes map[int]int    // seen status → count
	errorSigs   map[string]int // error body signature → count
	lastSeen    time.Time
}

// NewSeedInterestTracker creates an empty tracker.
func NewSeedInterestTracker() *SeedInterestTracker {
	return &SeedInterestTracker{
		scores: make(map[string]*seedStats),
	}
}

// RecordResponse records a replay result for a seed. Returns the interest
// increment applied (0 if no novelty found).
//
// Scoring:
//   - novel status code (not previously seen): +2.0
//   - novel error signature: +3.0
//   - total is bounded per call
func (t *SeedInterestTracker) RecordResponse(seedID string, statusCode int, errorBody string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	stats := t.ensureSeed(seedID)
	var increment float64

	// Novel status code?
	if _, seen := stats.statusCodes[statusCode]; !seen {
		stats.statusCodes[statusCode] = 1
		increment += 2.0
	}

	// Novel error body signature?
	if errorBody != "" {
		sig := errorSignature(errorBody)
		if _, seen := stats.errorSigs[sig]; !seen {
			stats.errorSigs[sig] = 1
			increment += 3.0
		}
	}

	stats.interest += increment
	stats.lastSeen = time.Now()
	return increment
}

// RecordFinding boosts interest for a seed that produced a finding.
func (t *SeedInterestTracker) RecordFinding(seedID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	stats := t.ensureSeed(seedID)
	stats.interest += 5.0
	stats.lastSeen = time.Now()
}

// GetInterest returns the current interest score for a seed (default 1.0).
func (t *SeedInterestTracker) GetInterest(seedID string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if stats, ok := t.scores[seedID]; ok {
		if stats.interest < 1.0 {
			return 1.0
		}
		return stats.interest
	}
	return 1.0
}

// NormalizedWeights returns a probability distribution for weighted random
// selection. Seeds with higher interest receive proportionally higher weight.
func (t *SeedInterestTracker) NormalizedWeights(seedIDs []string) []float64 {
	t.mu.Lock()
	defer t.mu.Unlock()

	weights := make([]float64, len(seedIDs))
	total := 0.0
	for i, id := range seedIDs {
		w := 1.0
		if stats, ok := t.scores[id]; ok && stats.interest > 0 {
			w = stats.interest
		}
		weights[i] = w
		total += w
	}
	if total > 0 {
		for i := range weights {
			weights[i] /= total
		}
	}
	return weights
}

// ensureSeed returns or creates stats for a seed (caller must hold mu).
func (t *SeedInterestTracker) ensureSeed(seedID string) *seedStats {
	if stats, ok := t.scores[seedID]; ok {
		return stats
	}
	stats := &seedStats{
		statusCodes: make(map[int]int),
		errorSigs:   make(map[string]int),
	}
	t.scores[seedID] = stats
	return stats
}

// errorSignature creates a truncated hash of an error body for dedup.
func errorSignature(body string) string {
	if len(body) == 0 {
		return ""
	}
	// Use first 512 bytes for the signature
	sample := body
	if len(sample) > 512 {
		sample = sample[:512]
	}
	h := sha256.Sum256([]byte(sample))
	return fmt.Sprintf("%x", h[:8])
}
