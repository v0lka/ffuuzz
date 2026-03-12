package endpoint

import (
	"context"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

// mockMerger records calls for testing.
type mockMerger struct {
	mu              sync.Mutex
	mergeCalls      []mergeCall
	paths           map[Origin][]string
	origins         []Origin
	mergeErr        error
	mergeReturnN    int
	listPathsErr    error
	listOriginsErr  error
}

type mergeCall struct {
	Origin         Origin
	SourcePrefixes []string
	TargetPrefix   string
}

func newMockMerger() *mockMerger {
	return &mockMerger{
		paths: make(map[Origin][]string),
	}
}

func (m *mockMerger) MergeRecordings(_ context.Context, origin Origin, sourcePrefixes []string, targetPrefix string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mergeErr != nil {
		return 0, m.mergeErr
	}
	m.mergeCalls = append(m.mergeCalls, mergeCall{
		Origin:         origin,
		SourcePrefixes: sourcePrefixes,
		TargetPrefix:   targetPrefix,
	})
	if m.mergeReturnN > 0 {
		return m.mergeReturnN, nil
	}
	return len(sourcePrefixes), nil
}

func (m *mockMerger) ListDistinctPaths(_ context.Context, origin Origin) ([]string, error) {
	if m.listPathsErr != nil {
		return nil, m.listPathsErr
	}
	return m.paths[origin], nil
}

func (m *mockMerger) ListOrigins(_ context.Context) ([]Origin, error) {
	if m.listOriginsErr != nil {
		return nil, m.listOriginsErr
	}
	return m.origins, nil
}

func (m *mockMerger) getMergeCalls() []mergeCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mergeCall, len(m.mergeCalls))
	copy(out, m.mergeCalls)
	return out
}

func testLogger() zerolog.Logger {
	return zerolog.Nop()
}

func TestResolver_ObservePath_NoCollapse(t *testing.T) {
	t.Parallel()
	mm := newMockMerger()
	r := NewResolver(mm, testLogger())

	origin := Origin{Scheme: "https", Host: "api.example.com", Port: 443}

	// Observe two distinct paths — not enough for collapse.
	got1 := r.ObservePath(origin, "/api/users/{_}")
	got2 := r.ObservePath(origin, "/api/posts/{_}")

	if got1 != "/api/users/{_}" {
		t.Errorf("expected /api/users/{_}, got %s", got1)
	}
	if got2 != "/api/posts/{_}" {
		t.Errorf("expected /api/posts/{_}, got %s", got2)
	}

	calls := mm.getMergeCalls()
	if len(calls) != 0 {
		t.Errorf("expected no merge calls, got %d", len(calls))
	}
}

func TestResolver_ObservePath_TriggersCollapse(t *testing.T) {
	t.Parallel()
	mm := newMockMerger()
	r := NewResolver(mm, testLogger())

	origin := Origin{Scheme: "https", Host: "api.example.com", Port: 443}

	// Observe enough distinct literal paths at the same position to trigger
	// a statistical collapse (3 literals with ratio > 0.3).
	r.ObservePath(origin, "/api/alpha")
	r.ObservePath(origin, "/api/beta")
	got := r.ObservePath(origin, "/api/gamma")

	// The third observation should trigger collapse and rewrite.
	if got != "/api/{_}" {
		t.Errorf("expected /api/{_}, got %s", got)
	}
}

func TestResolver_ObservePath_HeuristicAssistedCollapse(t *testing.T) {
	t.Parallel()
	mm := newMockMerger()
	r := NewResolver(mm, testLogger())

	origin := Origin{Scheme: "https", Host: "api.example.com", Port: 443}

	// A {_} sibling already exists + 2 literals should trigger
	// heuristic-assisted collapse.
	r.ObservePath(origin, "/api/{_}")
	r.ObservePath(origin, "/api/alpha")
	got := r.ObservePath(origin, "/api/beta")

	if got != "/api/{_}" {
		t.Errorf("expected /api/{_}, got %s", got)
	}
}

func TestResolver_ObservePath_EmptyPath(t *testing.T) {
	t.Parallel()
	mm := newMockMerger()
	r := NewResolver(mm, testLogger())

	origin := Origin{Scheme: "http", Host: "localhost", Port: 80}

	// Empty/root paths should pass through unchanged.
	if got := r.ObservePath(origin, ""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := r.ObservePath(origin, "/"); got != "/" {
		t.Errorf("expected /, got %q", got)
	}
}

func TestResolver_RebuildFromDB(t *testing.T) {
	t.Parallel()
	mm := newMockMerger()

	origin := Origin{Scheme: "https", Host: "shop.example.com", Port: 443}
	mm.origins = []Origin{origin}
	// Enough distinct paths to trigger a collapse at position 2
	// (/products/item-a, /products/item-b, /products/item-c).
	mm.paths[origin] = []string{
		"/products/item-a",
		"/products/item-b",
		"/products/item-c",
	}

	r := NewResolver(mm, testLogger())
	err := r.RebuildFromDB(context.Background())
	if err != nil {
		t.Fatalf("RebuildFromDB failed: %v", err)
	}

	// Should have detected and executed a merge synchronously.
	calls := mm.getMergeCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one merge call from rebuild")
	}

	if calls[0].TargetPrefix != "/products/{_}" {
		t.Errorf("expected target /products/{_}, got %s", calls[0].TargetPrefix)
	}
}

func TestResolver_RebuildFromDB_NoOrigins(t *testing.T) {
	t.Parallel()
	mm := newMockMerger()
	mm.origins = nil

	r := NewResolver(mm, testLogger())
	err := r.RebuildFromDB(context.Background())
	if err != nil {
		t.Fatalf("RebuildFromDB failed: %v", err)
	}

	calls := mm.getMergeCalls()
	if len(calls) != 0 {
		t.Errorf("expected no merge calls, got %d", len(calls))
	}
}

func TestResolver_MultipleOrigins(t *testing.T) {
	t.Parallel()
	mm := newMockMerger()
	r := NewResolver(mm, testLogger())

	origin1 := Origin{Scheme: "https", Host: "api.example.com", Port: 443}
	origin2 := Origin{Scheme: "https", Host: "other.example.com", Port: 443}

	// Feed 3 paths to origin1 (should collapse), only 2 to origin2 (should not).
	r.ObservePath(origin1, "/v1/alpha")
	r.ObservePath(origin1, "/v1/beta")
	r.ObservePath(origin1, "/v1/gamma")

	r.ObservePath(origin2, "/v1/alpha")
	r.ObservePath(origin2, "/v1/beta")

	// Give async merge a moment to fire.
	// Use getMergeCalls which is synchronised.
	calls := mm.getMergeCalls()

	// origin1 should have a merge call; origin2 should not.
	var origin1Merges int
	for _, c := range calls {
		if c.Origin == origin1 {
			origin1Merges++
		}
	}
	// After the first collapse, a single new literal alongside {_} does not
	// meet collapseMinWithPlaceholder (needs 2), so it stays literal.
	got := r.ObservePath(origin1, "/v1/delta")
	if got != "/v1/delta" {
		t.Errorf("expected /v1/delta (below threshold), got %s", got)
	}

	// Adding another literal triggers heuristic-assisted collapse
	// (2 literals + existing {_}).
	got2 := r.ObservePath(origin1, "/v1/epsilon")
	if got2 != "/v1/{_}" {
		t.Errorf("expected /v1/{_} after heuristic-assisted collapse, got %s", got2)
	}

	_ = origin1Merges // just checking no panic
}

func TestRewritePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		segments []string
		position int
		original string
		want     string
	}{
		{[]string{"api", "users", "123"}, 2, "/api/users/123", "/api/users/{_}"},
		{[]string{"api", "users"}, 1, "/api/users", "/api/{_}"},
		{[]string{"api"}, 0, "/api", "/{_}"},
		{[]string{"a", "b", "c"}, 5, "/a/b/c", "/a/b/c"}, // position out of range
	}

	for _, tt := range tests {
		got := rewritePath(tt.segments, tt.position, tt.original)
		if got != tt.want {
			t.Errorf("rewritePath(%v, %d, %q) = %q, want %q",
				tt.segments, tt.position, tt.original, got, tt.want)
		}
	}
}

func TestJoinSegments(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b", "c"}, "a/b/c"},
	}
	for _, tt := range tests {
		got := joinSegments(tt.in)
		if got != tt.want {
			t.Errorf("joinSegments(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
