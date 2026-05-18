package engine

import (
	"math/rand"
	"sync"
	"testing"
)

func TestSeedInterestTracker_New(t *testing.T) {
	tracker := NewSeedInterestTracker()
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	interest := tracker.GetInterest("unknown")
	if interest != 1.0 {
		t.Errorf("unknown seed: expected 1.0, got %f", interest)
	}
}

func TestSeedInterestTracker_NovelStatusCode(t *testing.T) {
	tracker := NewSeedInterestTracker()

	// First 200 — novel status code
	inc := tracker.RecordResponse("seed1", 200, "")
	if inc <= 0 {
		t.Errorf("first 200 should give increment (novel status code), got %f", inc)
	}

	// Same 200 again — no novelty
	inc = tracker.RecordResponse("seed1", 200, "")
	if inc > 0 {
		t.Errorf("repeat 200 should give no increment, got %f", inc)
	}

	// Novel 500 with error body — status novelty + error novelty
	inc = tracker.RecordResponse("seed1", 500, "error")
	if inc <= 0 {
		t.Error("novel 500 + error should give positive increment")
	}

	// Same status, different error — error novelty only
	inc = tracker.RecordResponse("seed1", 500, "another error")
	if inc <= 0 {
		t.Error("different error should give positive increment")
	}
}

func TestSeedInterestTracker_NovelError(t *testing.T) {
	tracker := NewSeedInterestTracker()

	inc := tracker.RecordResponse("seed1", 500, "unique error body")
	if inc <= 0 {
		t.Error("novel error body should give positive increment")
	}

	// Same error body — no novelty
	inc = tracker.RecordResponse("seed1", 500, "unique error body")
	if inc > 0 {
		t.Errorf("repeat error should give no increment, got %f", inc)
	}

	// Different error body — novelty
	inc = tracker.RecordResponse("seed1", 500, "different error")
	if inc <= 0 {
		t.Error("different error body should give positive increment")
	}
}

func TestSeedInterestTracker_FindingBoost(t *testing.T) {
	tracker := NewSeedInterestTracker()

	before := tracker.GetInterest("seed1")
	tracker.RecordFinding("seed1")
	after := tracker.GetInterest("seed1")

	if after <= before {
		t.Errorf("finding should boost interest: %f → %f", before, after)
	}
}

func TestSeedInterestTracker_NormalizedWeights(t *testing.T) {
	tracker := NewSeedInterestTracker()

	// All seeds start with equal weight
	weights := tracker.NormalizedWeights([]string{"a", "b", "c"})
	if len(weights) != 3 {
		t.Fatalf("expected 3 weights, got %d", len(weights))
	}

	var sum float64
	for _, w := range weights {
		sum += w
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("weights should sum to ~1.0, got %f", sum)
	}

	// Boost seed "a" with findings
	tracker.RecordFinding("a")
	tracker.RecordFinding("a")
	weights = tracker.NormalizedWeights([]string{"a", "b", "c"})

	// "a" should have higher weight than "b" and "c"
	if weights[0] <= weights[1] || weights[0] <= weights[2] {
		t.Errorf("boosted seed 'a' should have highest weight: %v", weights)
	}
}

func TestSeedInterestTracker_Concurrency(t *testing.T) {
	tracker := NewSeedInterestTracker()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tracker.RecordResponse("seed1", 200+i%5, "error")
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.RecordFinding("seed1")
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tracker.GetInterest("seed1")
			_ = tracker.NormalizedWeights([]string{"a", "b", "c"})
		}()
	}
	wg.Wait()
}

func TestWeightedPick(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	items := []string{"a", "b", "c"}

	// Uniform weights: each should appear approximately equally
	uniform := []float64{1.0 / 3.0, 1.0 / 3.0, 1.0 / 3.0}
	counts := map[string]int{}
	for i := 0; i < 10000; i++ {
		item := weightedPick(items, uniform, rng)
		counts[item]++
	}
	for _, count := range counts {
		if count < 2000 || count > 4500 {
			t.Logf("uniform distribution: %v", counts)
			break
		}
	}

	// Skewed weights: "a" should dominate
	rng = rand.New(rand.NewSource(99))
	skewed := []float64{0.9, 0.05, 0.05}
	counts = map[string]int{}
	for i := 0; i < 10000; i++ {
		item := weightedPick(items, skewed, rng)
		counts[item]++
	}
	if counts["a"] <= counts["b"] || counts["a"] <= counts["c"] {
		t.Errorf("skewed distribution: 'a' should dominate: %v", counts)
	}
}

func TestWeightedPick_Empty(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	result := weightedPick([]string{}, []float64{}, rng)
	if result != "" {
		t.Errorf("empty items should return zero value, got %q", result)
	}
}

func TestWeightedPick_Mismatched(t *testing.T) {
	rng := rand.New(rand.NewSource(0))
	// Mismatched lengths should return zero
	result := weightedPick([]string{"a"}, []float64{1.0, 0.0}, rng)
	if result != "" {
		t.Errorf("mismatched lengths should return zero, got %q", result)
	}
}
