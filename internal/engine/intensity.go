package engine

import (
	"strings"
	"sync"
)

// operatorPrefixes maps known operator name prefixes for intensity tracking.
var operatorPrefixes = []string{"uri", "header", "json", "param", "primitive"}

// IntensityTracker maintains per-operator finding/productivity statistics
// and computes adaptive intensity multipliers for campaign mutation.
// It is safe for concurrent use from multiple worker goroutines.
type IntensityTracker struct {
	mu        sync.Mutex
	operators map[string]*operatorStats
}

type operatorStats struct {
	applications int64 // total times this operator was tried
	findings     int64 // findings produced by this operator
}

// NewIntensityTracker creates a tracker pre-populated with known operator prefixes.
func NewIntensityTracker() *IntensityTracker {
	ops := make(map[string]*operatorStats, len(operatorPrefixes))
	for _, prefix := range operatorPrefixes {
		ops[prefix] = &operatorStats{}
	}
	return &IntensityTracker{operators: ops}
}

// RecordApplication increments the application counter for each operator prefix
// present in the given operator names (e.g. "uri:path_segment" → "uri").
func (t *IntensityTracker) RecordApplication(ops []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, op := range ops {
		prefix := operatorPrefix(op)
		if stats, ok := t.operators[prefix]; ok {
			stats.applications++
		}
	}
}

// RecordFinding increments the finding counter for each operator prefix.
func (t *IntensityTracker) RecordFinding(ops []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, op := range ops {
		prefix := operatorPrefix(op)
		if stats, ok := t.operators[prefix]; ok {
			stats.findings++
		}
	}
}

// GetMultiplier returns the current intensity multiplier for an operator prefix.
//
// Formula:
//
//	productivity = findings / max(1, applications)
//	boost        = min(productivity * 1.5, 1.0)  // cap at +100%
//	exploration  = 0.5 if applications < 10, else 0
//	multiplier   = 1.0 + boost + exploration      // max 2.5x
func (t *IntensityTracker) GetMultiplier(prefix string) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	stats, ok := t.operators[prefix]
	if !ok {
		return 1.0
	}
	apps := stats.applications
	baseDiv := apps
	if baseDiv < 1 {
		baseDiv = 1
	}
	productivity := float64(stats.findings) / float64(baseDiv)
	boost := productivity * 1.5
	if boost > 1.0 {
		boost = 1.0
	}
	var exploration float64
	if apps < 10 {
		exploration = 0.5
	}
	multiplier := 1.0 + boost + exploration
	if multiplier > 2.5 {
		multiplier = 2.5
	}
	return multiplier
}

// operatorPrefix extracts the prefix from a multi-segment operator name.
// e.g. "uri:path_segment" → "uri", "header:add" → "header", "seq:drop" → "seq".
func operatorPrefix(op string) string {
	if idx := strings.IndexByte(op, ':'); idx >= 0 {
		return op[:idx]
	}
	return op
}
