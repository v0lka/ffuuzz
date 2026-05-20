package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/model"
)

// twoEndpointSeeds returns two seed sessions, one per endpoint, used by the
// planner integration tests below.
func twoEndpointSeeds() []model.RecordingSession {
	return []model.RecordingSession{
		{
			ID: "s1",
			Entries: []model.Exchange{
				{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/api/users"}, Response: model.ResponseData{Status: 200}},
			},
		},
		{
			ID: "s2",
			Entries: []model.Exchange{
				{RequestID: "r2", Request: model.RequestData{Method: "POST", Path: "/api/orders"}, Response: model.ResponseData{Status: 201}},
			},
		},
	}
}

// TestRunCampaign_FairCoverageAcrossEndpoints verifies that with the planner
// engaged (SequenceShare=0) every endpoint is targeted at least once across
// many tests rather than concentrating on a single seed.
func TestRunCampaign_FairCoverageAcrossEndpoints(t *testing.T) {
	var hits sync.Map // path -> *atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, _ := hits.LoadOrStore(r.URL.Path, new(atomic.Int64))
		v.(*atomic.Int64).Add(1)
		w.WriteHeader(200)
	}))
	defer backend.Close()

	seeds := twoEndpointSeeds()
	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{
			Workers:       2,
			RPS:           1000,
			MaxTests:      40,
			ReqTimeoutMs:  500,
			SequenceShare: 0, // pure planner mode
		},
	}

	planner, err := NewEndpointPlanner(seeds, nil, 0, 1)
	if err != nil {
		t.Fatalf("planner: %v", err)
	}

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, t.TempDir(), zerolog.Nop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	e.runCampaign(ctx, "camp-fair", cfg, seeds, map[string]*anomaly.BaselineEntry{}, planner)

	usersHits := loadCounter(&hits, "/api/users")
	ordersHits := loadCounter(&hits, "/api/orders")
	if usersHits == 0 || ordersHits == 0 {
		t.Fatalf("expected both endpoints to receive traffic, got users=%d orders=%d", usersHits, ordersHits)
	}
}

// TestRunCampaign_RespectsEndpointDisabled verifies that an endpoint marked
// Disabled in the override list never receives traffic when running pure
// planner mode (SequenceShare=0).
func TestRunCampaign_RespectsEndpointDisabled(t *testing.T) {
	var hits sync.Map
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, _ := hits.LoadOrStore(r.URL.Path, new(atomic.Int64))
		v.(*atomic.Int64).Add(1)
		w.WriteHeader(200)
	}))
	defer backend.Close()

	seeds := twoEndpointSeeds()

	overrides := []model.EndpointWeightOverride{
		{Method: "POST", Path: endpoint.NormalizePath("/api/orders"), Disabled: true},
	}
	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{
			Workers:         2,
			RPS:             1000,
			MaxTests:        20,
			ReqTimeoutMs:    500,
			SequenceShare:   0,
			EndpointWeights: overrides,
		},
	}

	planner, err := NewEndpointPlanner(seeds, overrides, 0, 1)
	if err != nil {
		t.Fatalf("planner: %v", err)
	}

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, t.TempDir(), zerolog.Nop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e.runCampaign(ctx, "camp-disabled", cfg, seeds, map[string]*anomaly.BaselineEntry{}, planner)

	if loadCounter(&hits, "/api/orders") != 0 {
		t.Errorf("disabled endpoint /api/orders received traffic")
	}
	if loadCounter(&hits, "/api/users") == 0 {
		t.Errorf("enabled endpoint /api/users got no traffic")
	}
}

// TestRunCampaign_SequenceShareSession verifies that with SequenceShare=1.0
// every task runs in session mode (which mutates every exchange in a
// session) and that the planner is not consulted for selection.
func TestRunCampaign_SequenceShareSession(t *testing.T) {
	var hits sync.Map
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, _ := hits.LoadOrStore(r.URL.Path, new(atomic.Int64))
		v.(*atomic.Int64).Add(1)
		w.WriteHeader(200)
	}))
	defer backend.Close()

	// Two-exchange session forces session mode to hit both endpoints when
	// the entire session is replayed.
	seeds := []model.RecordingSession{
		{
			ID: "multi",
			Entries: []model.Exchange{
				{RequestID: "a", Request: model.RequestData{Method: "GET", Path: "/api/a"}, Response: model.ResponseData{Status: 200}},
				{RequestID: "b", Request: model.RequestData{Method: "GET", Path: "/api/b"}, Response: model.ResponseData{Status: 200}},
			},
		},
	}

	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{
			Workers:       1,
			RPS:           1000,
			MaxTests:      4,
			ReqTimeoutMs:  500,
			SequenceShare: 1.0,
		},
	}

	planner, err := NewEndpointPlanner(seeds, nil, 0, 1)
	if err != nil {
		t.Fatalf("planner: %v", err)
	}

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, t.TempDir(), zerolog.Nop())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e.runCampaign(ctx, "camp-seq", cfg, seeds, map[string]*anomaly.BaselineEntry{}, planner)

	// Session mode replays every exchange in the session, so both /api/a
	// and /api/b should be hit.
	if loadCounter(&hits, "/api/a") == 0 || loadCounter(&hits, "/api/b") == 0 {
		t.Errorf("expected both /api/a and /api/b to be hit in session mode, got a=%d b=%d",
			loadCounter(&hits, "/api/a"), loadCounter(&hits, "/api/b"))
	}
}

func loadCounter(m *sync.Map, key string) int64 {
	v, ok := m.Load(key)
	if !ok {
		return 0
	}
	return v.(*atomic.Int64).Load()
}
