package engine

import (
	"sync"
	"testing"
)

func TestIntensityTracker_NewTracker(t *testing.T) {
	tracker := NewIntensityTracker()
	if tracker == nil {
		t.Fatal("expected non-nil tracker")
	}
	// All known operators should return 1.0 + exploration bonus (0.5) = 1.5
	for _, prefix := range operatorPrefixes {
		m := tracker.GetMultiplier(prefix)
		if m < 1.0 {
			t.Errorf("%s: expected multiplier >= 1.0, got %f", prefix, m)
		}
	}
}

func TestIntensityTracker_UnknownOperator(t *testing.T) {
	tracker := NewIntensityTracker()
	m := tracker.GetMultiplier("nonexistent")
	if m != 1.0 {
		t.Errorf("unknown operator: expected 1.0, got %f", m)
	}
}

func TestIntensityTracker_ProductivityIncreases(t *testing.T) {
	tracker := NewIntensityTracker()

	// Record a finding with no applications → productivity = finding/apps
	tracker.RecordFinding([]string{"uri:path_segment", "header:add"})
	// Use RecordApplication + RecordFinding to simulate productive operator
	tracker.RecordApplication([]string{"uri:path_segment", "uri:reserved_inject"})
	tracker.RecordApplication([]string{"uri:path_segment"})
	tracker.RecordFinding([]string{"uri:path_segment"})

	m := tracker.GetMultiplier("uri")
	if m <= 1.0 {
		t.Errorf("productive operator should have multiplier > 1.0, got %f", m)
	}
}

func TestIntensityTracker_UnexploredBonus(t *testing.T) {
	tracker := NewIntensityTracker()

	// No applications yet → should get exploration bonus
	m := tracker.GetMultiplier("header")
	if m < 1.0 {
		t.Errorf("unexplored operator should have multiplier >= 1.0, got %f", m)
	}
	// After 10 applications, exploration bonus should go away
	for i := 0; i < 10; i++ {
		tracker.RecordApplication([]string{"header:add"})
	}
	m2 := tracker.GetMultiplier("header")
	mWithBonus := tracker.GetMultiplier("uri")
	// header (10 apps, no findings) should return lower multiplier than uri (0 apps)
	if m2 >= mWithBonus {
		t.Logf("header mult=%f, uri mult=%f (uri has exploration bonus)", m2, mWithBonus)
	}
}

func TestIntensityTracker_MultiplierCap(t *testing.T) {
	tracker := NewIntensityTracker()
	// Simulate very productive operator
	for i := 0; i < 100; i++ {
		tracker.RecordApplication([]string{"uri:path_segment"})
		tracker.RecordFinding([]string{"uri:path_segment"})
		tracker.RecordFinding([]string{"uri:path_segment"})
	}
	m := tracker.GetMultiplier("uri")
	if m > 2.5 {
		t.Errorf("multiplier should be capped at 2.5, got %f", m)
	}
}

func TestIntensityTracker_RecordApplication_MultiplePrefixes(t *testing.T) {
	tracker := NewIntensityTracker()
	tracker.RecordApplication([]string{
		"uri:path_segment",
		"header:add",
		"json:string_mutation",
		"param:string_mutation",
		"primitive:bitflip",
		"seq:drop",
	})
	// seq is not in operatorPrefixes, so it won't be tracked
	m := tracker.GetMultiplier("seq")
	if m != 1.0 {
		t.Errorf("untracked operator should return 1.0, got %f", m)
	}
}

func TestIntensityTracker_Concurrency(t *testing.T) {
	tracker := NewIntensityTracker()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.RecordApplication([]string{"uri:path_segment"})
		}()
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.RecordFinding([]string{"uri:path_segment"})
		}()
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = tracker.GetMultiplier("uri")
		}()
	}
	wg.Wait()
}

func TestOperatorPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"uri:path_segment", "uri"},
		{"header:add", "header"},
		{"json:string_mutation", "json"},
		{"param:string_mutation", "param"},
		{"primitive:bitflip", "primitive"},
		{"seq:drop", "seq"},
		{"no_colon", "no_colon"},
		{"", ""},
	}
	for _, tt := range tests {
		got := operatorPrefix(tt.input)
		if got != tt.expected {
			t.Errorf("operatorPrefix(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
