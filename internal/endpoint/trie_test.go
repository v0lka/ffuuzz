package endpoint

import (
	"sort"
	"testing"
)

func TestTrieNode_Observe(t *testing.T) {
	root := newTrieNode()
	root.observe([]string{"api", "users", "blue"})
	root.observe([]string{"api", "users", "red"})

	// root -> api -> users -> {blue, red}
	api := root.children["api"]
	if api == nil {
		t.Fatal("expected 'api' child")
	}
	users := api.children["users"]
	if users == nil {
		t.Fatal("expected 'users' child")
	}
	if len(users.children) != 2 {
		t.Fatalf("expected 2 children under 'users', got %d", len(users.children))
	}
	if users.observationCount != 2 {
		t.Errorf("expected observationCount=2 for 'users', got %d", users.observationCount)
	}
}

func TestTrieNode_CheckCollapse_NoCollapse(t *testing.T) {
	root := newTrieNode()
	// Only 2 unique children — below threshold of 3.
	root.observe([]string{"api", "widgets", "blue"})
	root.observe([]string{"api", "widgets", "red"})

	_, found := root.checkCollapse()
	if found {
		t.Error("expected no collapse with only 2 unique children")
	}
}

func TestTrieNode_CheckCollapse_StatisticalCollapse(t *testing.T) {
	root := newTrieNode()
	// 3 unique values at position 2, all with 1 observation each.
	// Ratio = 3/3 = 1.0 > 0.3 → collapse.
	root.observe([]string{"api", "widgets", "blue"})
	root.observe([]string{"api", "widgets", "red"})
	root.observe([]string{"api", "widgets", "green"})

	result, found := root.checkCollapse()
	if !found {
		t.Fatal("expected collapse to be detected")
	}
	if result.Position != 2 {
		t.Errorf("expected collapse at position 2, got %d", result.Position)
	}
	if len(result.ParentSegments) != 2 || result.ParentSegments[0] != "api" || result.ParentSegments[1] != "widgets" {
		t.Errorf("unexpected parent segments: %v", result.ParentSegments)
	}
}

func TestTrieNode_CheckCollapse_HighTrafficNoCollapse(t *testing.T) {
	root := newTrieNode()
	// Simulate high traffic: 100 observations through root, but only 3
	// unique children. Ratio = 3/100 = 0.03 < 0.3 → no collapse.
	for i := 0; i < 100; i++ {
		root.observe([]string{"api"})
	}
	root.observe([]string{"docs"})
	root.observe([]string{"admin"})

	_, found := root.checkCollapse()
	if found {
		t.Error("expected no collapse for high-traffic root with few unique children")
	}
}

func TestTrieNode_CheckCollapse_HeuristicAssisted(t *testing.T) {
	root := newTrieNode()
	// Simulate: heuristic already created a {_} child, and 2 literal children exist.
	root.observe([]string{"api", "users", Placeholder})
	root.observe([]string{"api", "users", "me"})
	root.observe([]string{"api", "users", "profile"})

	result, found := root.checkCollapse()
	if !found {
		t.Fatal("expected heuristic-assisted collapse")
	}
	if result.Position != 2 {
		t.Errorf("expected collapse at position 2, got %d", result.Position)
	}
}

func TestTrieNode_Collapse(t *testing.T) {
	root := newTrieNode()
	root.observe([]string{"api", "widgets", "blue"})
	root.observe([]string{"api", "widgets", "red"})
	root.observe([]string{"api", "widgets", "green"})

	collapsed := root.collapse([]string{"api", "widgets"})
	sort.Strings(collapsed)

	if len(collapsed) != 3 {
		t.Fatalf("expected 3 collapsed keys, got %d: %v", len(collapsed), collapsed)
	}
	expected := []string{"blue", "green", "red"}
	for i, key := range collapsed {
		if key != expected[i] {
			t.Errorf("collapsed[%d] = %q, want %q", i, key, expected[i])
		}
	}

	// After collapse, "widgets" should have only {_} child.
	widgets := root.children["api"].children["widgets"]
	if len(widgets.children) != 1 {
		t.Errorf("expected 1 child under widgets after collapse, got %d", len(widgets.children))
	}
	if _, ok := widgets.children[Placeholder]; !ok {
		t.Error("expected {_} child under widgets after collapse")
	}
}

func TestTrieNode_Collapse_WithExistingPlaceholder(t *testing.T) {
	root := newTrieNode()
	root.observe([]string{"api", "users", Placeholder})
	root.observe([]string{"api", "users", "me"})
	root.observe([]string{"api", "users", "profile"})

	collapsed := root.collapse([]string{"api", "users"})
	sort.Strings(collapsed)

	if len(collapsed) != 2 {
		t.Fatalf("expected 2 collapsed keys, got %d: %v", len(collapsed), collapsed)
	}
	if collapsed[0] != "me" || collapsed[1] != "profile" {
		t.Errorf("unexpected collapsed keys: %v", collapsed)
	}

	// After collapse, only {_} should remain.
	users := root.children["api"].children["users"]
	if len(users.children) != 1 {
		t.Errorf("expected 1 child, got %d", len(users.children))
	}
}

func TestCollapsedPatterns(t *testing.T) {
	sources, target := collapsedPatterns(
		[]string{"api", "widgets"},
		[]string{"blue", "red", "green"},
	)
	sort.Strings(sources)

	expectedSources := []string{"/api/widgets/blue", "/api/widgets/green", "/api/widgets/red"}
	if len(sources) != 3 {
		t.Fatalf("expected 3 source patterns, got %d", len(sources))
	}
	for i, s := range sources {
		if s != expectedSources[i] {
			t.Errorf("source[%d] = %q, want %q", i, s, expectedSources[i])
		}
	}
	if target != "/api/widgets/{_}" {
		t.Errorf("target = %q, want %q", target, "/api/widgets/{_}")
	}
}

func TestCollapsedPatterns_RootLevel(t *testing.T) {
	sources, target := collapsedPatterns(
		nil,
		[]string{"a", "b", "c"},
	)
	sort.Strings(sources)

	if target != "/{_}" {
		t.Errorf("target = %q, want %q", target, "/{_}")
	}
	expectedSources := []string{"/a", "/b", "/c"}
	for i, s := range sources {
		if s != expectedSources[i] {
			t.Errorf("source[%d] = %q, want %q", i, s, expectedSources[i])
		}
	}
}

func TestTrieNode_CheckCollapse_Cascading(t *testing.T) {
	root := newTrieNode()
	// Level 2 has many unique values → collapse there first.
	root.observe([]string{"api", "widgets", "blue", "detail"})
	root.observe([]string{"api", "widgets", "red", "detail"})
	root.observe([]string{"api", "widgets", "green", "detail"})

	result, found := root.checkCollapse()
	if !found {
		t.Fatal("expected collapse")
	}
	if result.Position != 2 {
		t.Errorf("expected collapse at position 2, got %d", result.Position)
	}

	// Perform the collapse.
	root.collapse(result.ParentSegments)

	// After collapse, {_} is the only child under widgets.
	// Check that no further collapse is needed.
	_, found = root.checkCollapse()
	if found {
		t.Error("expected no further collapse after first merge")
	}
}
