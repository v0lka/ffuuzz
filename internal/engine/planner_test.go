package engine

import (
	"sync"
	"testing"

	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/model"
)

// makeSession builds a recording session with one exchange per provided
// (method, path) pair. SessionIdx is set by the caller's slice ordering.
func makeSession(pairs ...[2]string) model.RecordingSession {
	entries := make([]model.Exchange, len(pairs))
	for i, p := range pairs {
		entries[i] = model.Exchange{
			Request: model.RequestData{Method: p[0], Path: p[1]},
		}
	}
	return model.RecordingSession{Entries: entries}
}

func TestNewEndpointPlanner_AllDisabled_Errors(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession([2]string{"GET", "/api/users"}),
	}
	overrides := []model.EndpointWeightOverride{
		{Method: "GET", Path: "/api/users", Disabled: true},
	}
	if _, err := NewEndpointPlanner(seeds, overrides, 0, 1); err == nil {
		t.Fatalf("expected error when all endpoints are disabled, got nil")
	}
}

func TestNewEndpointPlanner_NormalizesAcrossSessions(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession([2]string{"GET", "/api/users/1"}),
		makeSession([2]string{"GET", "/api/users/2"}),
	}
	p, err := NewEndpointPlanner(seeds, nil, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.keys) != 1 {
		t.Fatalf("expected 1 normalised key, got %d: %v", len(p.keys), p.keys)
	}
	want := endpoint.NewKey("GET", "/api/users/{_}")
	if p.keys[0] != want {
		t.Errorf("got key %v, want %v", p.keys[0], want)
	}
	if len(p.sessions[want]) != 2 {
		t.Errorf("expected 2 exchange refs, got %d", len(p.sessions[want]))
	}
}

func TestPlanner_FloorPhase_VisitsEachKey(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession(
			[2]string{"GET", "/api/a"},
			[2]string{"GET", "/api/b"},
			[2]string{"GET", "/api/c"},
		),
	}
	const floor = 3
	p, err := NewEndpointPlanner(seeds, nil, floor, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	counts := make(map[endpoint.Key]int)
	for i := 0; i < floor*len(p.keys); i++ {
		k, _ := p.Pick()
		counts[k]++
	}
	for _, k := range p.keys {
		if counts[k] < floor {
			t.Errorf("key %v: got %d picks, want >= floor=%d", k, counts[k], floor)
		}
	}
}

func TestPlanner_UCB_NewKeyWins(t *testing.T) {
	// Seed with two endpoints; warm one with picks/rewards, then introduce a
	// fresh planner where the second key has n=0 — UCB must pick it first.
	seeds := []model.RecordingSession{
		makeSession(
			[2]string{"GET", "/api/warm"},
			[2]string{"GET", "/api/fresh"},
		),
	}
	p, err := NewEndpointPlanner(seeds, nil, 0, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Manually warm one key so n>0 and its mean is non-trivial.
	warm := endpoint.NewKey("GET", "/api/warm")
	p.stats[warm].n = 5
	p.stats[warm].rewardSum = 10
	p.totalN = 5

	k, _ := p.Pick()
	want := endpoint.NewKey("GET", "/api/fresh")
	if k != want {
		t.Errorf("expected new key %v to win UCB, got %v", want, k)
	}
}

func TestPlanner_UserWeight_BiasesSelection(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession(
			[2]string{"GET", "/api/a"},
			[2]string{"GET", "/api/b"},
		),
	}
	overrides := []model.EndpointWeightOverride{
		{Method: "GET", Path: "/api/a", Weight: 10.0},
	}
	p, err := NewEndpointPlanner(seeds, overrides, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warm both keys equally so UCB scores are comparable.
	a := endpoint.NewKey("GET", "/api/a")
	b := endpoint.NewKey("GET", "/api/b")
	p.stats[a].n = 1
	p.stats[a].rewardSum = 1
	p.stats[b].n = 1
	p.stats[b].rewardSum = 1
	p.totalN = 2

	const trials = 500
	counts := map[endpoint.Key]int{}
	for i := 0; i < trials; i++ {
		k, _ := p.Pick()
		counts[k]++
	}
	if counts[a] <= counts[b] {
		t.Errorf("expected weighted key %v to dominate %v: a=%d b=%d",
			a, b, counts[a], counts[b])
	}
}

func TestPlanner_Disabled_Excluded(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession(
			[2]string{"GET", "/api/keep"},
			[2]string{"GET", "/api/skip"},
		),
	}
	overrides := []model.EndpointWeightOverride{
		{Method: "GET", Path: "/api/skip", Disabled: true},
	}
	p, err := NewEndpointPlanner(seeds, overrides, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skip := endpoint.NewKey("GET", "/api/skip")
	for i := 0; i < 100; i++ {
		k, _ := p.Pick()
		if k == skip {
			t.Fatalf("disabled key %v was picked at iteration %d", skip, i)
		}
	}
}

func TestPlanner_Disabled_WildcardMethod(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession(
			[2]string{"GET", "/api/x"},
			[2]string{"POST", "/api/x"},
			[2]string{"GET", "/api/y"},
		),
	}
	overrides := []model.EndpointWeightOverride{
		{Method: "", Path: "/api/x", Disabled: true},
	}
	p, err := NewEndpointPlanner(seeds, overrides, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.keys) != 1 {
		t.Errorf("expected only /api/y to remain, got keys=%v", p.keys)
	}
}

func TestPlanner_Snapshot(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession(
			[2]string{"GET", "/api/a"},
			[2]string{"GET", "/api/b"},
		),
	}
	p, err := NewEndpointPlanner(seeds, nil, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	a := endpoint.NewKey("GET", "/api/a")
	for i := 0; i < 3; i++ {
		p.Pick()
	}
	p.Reward(a, 4)
	p.RecordFinding(a)

	snap := p.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 progress rows, got %d", len(snap))
	}
	var got EndpointProgress
	for _, r := range snap {
		if r.Key == a {
			got = r
			break
		}
	}
	if got.Tests == 0 {
		t.Errorf("expected positive Tests for %v, got 0", a)
	}
	if got.Findings != 1 {
		t.Errorf("expected 1 finding, got %d", got.Findings)
	}
}

func TestPlanner_LastPickedKey(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession([2]string{"GET", "/api/only"}),
	}
	p, err := NewEndpointPlanner(seeds, nil, 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	k, _ := p.Pick()
	if got := p.LastPickedKey(); got != k {
		t.Errorf("LastPickedKey() = %v, want %v", got, k)
	}
}

func TestPlanner_Concurrency(t *testing.T) {
	seeds := []model.RecordingSession{
		makeSession(
			[2]string{"GET", "/api/a"},
			[2]string{"GET", "/api/b"},
			[2]string{"GET", "/api/c"},
		),
	}
	p, err := NewEndpointPlanner(seeds, nil, 0, 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const goroutines = 8
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				k, _ := p.Pick()
				p.Reward(k, 1)
				if i%10 == 0 {
					p.RecordFinding(k)
				}
				_ = p.Snapshot()
			}
		}()
	}
	wg.Wait()

	totalTests := int64(0)
	for _, r := range p.Snapshot() {
		totalTests += r.Tests
	}
	if want := int64(goroutines * iters); totalTests != want {
		t.Errorf("totalTests = %d, want %d", totalTests, want)
	}
}
