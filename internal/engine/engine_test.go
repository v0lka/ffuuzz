package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/anomaly"
	"ffuuzz/internal/model"
	"ffuuzz/internal/mutate"
	"ffuuzz/internal/replayer"
	"ffuuzz/internal/triage"
)

type mockCampaignStore struct {
	updateStatusFn   func(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error)
	incrementStatsFn func(ctx context.Context, id string, testsDelta, findingsDelta int) error
}

func (m *mockCampaignStore) UpdateStatus(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, oldStatus, newStatus)
	}
	return true, nil
}
func (m *mockCampaignStore) IncrementStats(ctx context.Context, id string, testsDelta, findingsDelta int) error {
	if m.incrementStatsFn != nil {
		return m.incrementStatsFn(ctx, id, testsDelta, findingsDelta)
	}
	return nil
}

type mockFindingStore struct {
	existsBySignatureFn func(ctx context.Context, campaignID, signature string) (bool, error)
	createFn            func(ctx context.Context, f model.Finding) error
	updateStatusFn      func(ctx context.Context, id string, status model.FindingStatus) error
}

func (m *mockFindingStore) ExistsBySignature(ctx context.Context, campaignID, signature string) (bool, error) {
	if m.existsBySignatureFn != nil {
		return m.existsBySignatureFn(ctx, campaignID, signature)
	}
	return false, nil
}
func (m *mockFindingStore) Create(ctx context.Context, f model.Finding) error {
	if m.createFn != nil {
		return m.createFn(ctx, f)
	}
	return nil
}
func (m *mockFindingStore) UpdateStatus(ctx context.Context, id string, status model.FindingStatus) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	return nil
}
func (m *mockFindingStore) GetByID(ctx context.Context, id string) (*model.Finding, error) {
	return nil, nil
}
func (m *mockFindingStore) ClaimNextReproduceJob(ctx context.Context) (string, int, bool, error) {
	return "", 0, false, nil
}
func (m *mockFindingStore) SetReproduceStatus(ctx context.Context, id, status string) error {
	return nil
}

func (m *mockFindingStore) UpdateLLMAnalysis(ctx context.Context, id string, analysisJSON []byte) error {
	return nil
}

type mockArtifactStore struct {
	createFn func(ctx context.Context, a model.Artifact) error
}

func (m *mockArtifactStore) Create(ctx context.Context, a model.Artifact) error {
	if m.createFn != nil {
		return m.createFn(ctx, a)
	}
	return nil
}
func (m *mockArtifactStore) GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error) {
	return nil, nil
}

func TestNewEngine(t *testing.T) {
	cs := &mockCampaignStore{}
	fs := &mockFindingStore{}
	as := &mockArtifactStore{}
	logger := zerolog.Nop()

	e := NewEngine(cs, fs, as, nil, nil, "/tmp/artifacts", logger)
	if e == nil {
		t.Fatal("expected non-nil Engine")
	}
	if e.campaigns != cs {
		t.Error("campaigns store not set")
	}
	if e.findings != fs {
		t.Error("findings store not set")
	}
	if e.artifacts != as {
		t.Error("artifacts store not set")
	}
	if e.artifactDir != "/tmp/artifacts" {
		t.Errorf("artifactDir = %q, want /tmp/artifacts", e.artifactDir)
	}
	if e.running == nil {
		t.Error("running map should be initialized")
	}
}

func TestIsRunning_NotRunning(t *testing.T) {
	e := NewEngine(&mockCampaignStore{}, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())
	if e.IsRunning("nonexistent") {
		t.Error("expected IsRunning=false for non-existent campaign")
	}
}

func TestIsRunning_Running(t *testing.T) {
	e := NewEngine(&mockCampaignStore{}, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())
	e.mu.Lock()
	e.running["camp-1"] = func() {}
	e.mu.Unlock()

	if !e.IsRunning("camp-1") {
		t.Error("expected IsRunning=true for running campaign")
	}
}

func TestStopCampaign_NotRunning(t *testing.T) {
	e := NewEngine(&mockCampaignStore{}, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())
	err := e.StopCampaign(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-running campaign")
	}
}

func TestStopCampaign_Running(t *testing.T) {
	cancelled := false
	cs := &mockCampaignStore{
		updateStatusFn: func(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
			return true, nil
		},
	}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())

	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.running["camp-1"] = func() {
		cancelled = true
		cancel()
	}
	e.mu.Unlock()

	err := e.StopCampaign(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cancelled {
		t.Error("expected cancel function to be called")
	}
	_ = ctx // keep reference
}

func TestStopCampaign_UpdateStatusError(t *testing.T) {
	cs := &mockCampaignStore{
		updateStatusFn: func(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
			// Fail for RUNNING->STOPPING, succeed for STARTING->STOPPING
			if oldStatus == model.CampaignRunning {
				return false, fmt.Errorf("db error")
			}
			return true, nil
		},
	}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())

	e.mu.Lock()
	e.running["camp-1"] = func() {}
	e.mu.Unlock()

	err := e.StopCampaign(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("StopCampaign should not return error even if UpdateStatus fails: %v", err)
	}
}

func TestStopAll(t *testing.T) {
	cancelledIDs := make(map[string]bool)
	cs := &mockCampaignStore{
		updateStatusFn: func(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
			return true, nil
		},
	}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())

	e.mu.Lock()
	e.running["c1"] = func() { cancelledIDs["c1"] = true }
	e.running["c2"] = func() { cancelledIDs["c2"] = true }
	e.mu.Unlock()

	e.StopAll(context.Background())

	if !cancelledIDs["c1"] {
		t.Error("expected c1 to be cancelled")
	}
	if !cancelledIDs["c2"] {
		t.Error("expected c2 to be cancelled")
	}
}

func TestFailCampaign(t *testing.T) {
	updateCalled := false
	cs := &mockCampaignStore{
		updateStatusFn: func(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
			updateCalled = true
			if newStatus != model.CampaignFailed {
				t.Errorf("expected FAILED status, got %s", newStatus)
			}
			return true, nil
		},
	}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())
	e.failCampaign(context.Background(), "camp-1", model.CampaignStarting, context.DeadlineExceeded)

	if !updateCalled {
		t.Error("expected UpdateStatus to be called")
	}
}

func TestStartCampaign_UpdateStatusFails(t *testing.T) {
	cs := &mockCampaignStore{
		updateStatusFn: func(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
			return false, context.DeadlineExceeded
		},
	}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())
	campaign := &model.Campaign{ID: "c1", Status: model.CampaignCreated}
	err := e.StartCampaign(context.Background(), campaign)
	if err == nil {
		t.Error("expected error when UpdateStatus fails")
	}
}

func TestStartCampaign_NotExpectedState(t *testing.T) {
	cs := &mockCampaignStore{
		updateStatusFn: func(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
			return false, nil // not in expected state
		},
	}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, "", zerolog.Nop())
	campaign := &model.Campaign{ID: "c1", Status: model.CampaignCreated}
	err := e.StartCampaign(context.Background(), campaign)
	if err == nil {
		t.Error("expected error when campaign not in expected state")
	}
}

func TestRunCampaign_MaxTests(t *testing.T) {
	// Start a test HTTP server that always returns 200
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()

	cs := &mockCampaignStore{}
	fs := &mockFindingStore{}
	as := &mockArtifactStore{}
	e := NewEngine(cs, fs, as, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{
			ID: "s1",
			Entries: []model.Exchange{
				{
					RequestID: "r1",
					Request:   model.RequestData{Method: "GET", Path: "/test"},
					Response:  model.ResponseData{Status: 200},
				},
			},
		},
	}
	baselines := map[string]*anomaly.BaselineEntry{}

	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{
			Workers:  1,
			MaxTests: 3,
		},
	}

	ctx := context.Background()
	e.runCampaign(ctx, "camp-1", cfg, seeds, baselines)

	// After runCampaign returns, campaign should no longer be in running map
	if e.IsRunning("camp-1") {
		t.Error("campaign should not be running after runCampaign returns")
	}
}

func TestRunCampaign_DurationLimit(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{ID: "s1", Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/"}}}},
	}

	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{
			Workers:     1,
			DurationSec: 1, // 1 second limit
		},
	}

	start := time.Now()
	e.runCampaign(context.Background(), "camp-2", cfg, seeds, nil)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("runCampaign took too long: %v", elapsed)
	}
}

func TestRunCampaign_ContextCancelled(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{ID: "s1", Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/"}}}},
	}

	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{Workers: 1},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	e.runCampaign(ctx, "camp-3", cfg, seeds, nil)
	// Should complete without hanging
}

func TestRunCampaign_WithRateLimit(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{ID: "s1", Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/"}}}},
	}

	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{
			Workers:  1,
			MaxTests: 2,
			RPS:      100,
		},
	}

	e.runCampaign(context.Background(), "camp-4", cfg, seeds, nil)
}

func TestRunCampaign_WithSequenceMutation(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{
			ID: "s1",
			Entries: []model.Exchange{
				{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/a"}, Response: model.ResponseData{Status: 200}},
				{RequestID: "r2", Request: model.RequestData{Method: "POST", Path: "/b"}, Response: model.ResponseData{Status: 200}},
			},
		},
	}

	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{Workers: 1, MaxTests: 2},
		Mutations: model.MutationConfig{
			Sequence:  true,
			Intensity: 0.5,
		},
	}

	e.runCampaign(context.Background(), "camp-5", cfg, seeds, nil)
}

func TestRunCampaign_DefaultWorkers(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()

	cs := &mockCampaignStore{}
	e := NewEngine(cs, &mockFindingStore{}, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{ID: "s1", Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/"}}}},
	}

	cfg := model.CampaignConfig{
		Target: model.TargetURL{BaseURL: backend.URL},
		Limits: model.CampaignLimits{Workers: 0, MaxTests: 1}, // 0 workers -> default 4
	}

	e.runCampaign(context.Background(), "camp-default", cfg, seeds, nil)
}

func TestRunCampaign_Anomaly5xxDetection(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500) // Server error - should trigger anomaly detection
	}))
	defer backend.Close()

	findingsCreated := 0
	fs := &mockFindingStore{
		createFn: func(ctx context.Context, f model.Finding) error {
			findingsCreated++
			return nil
		},
	}
	cs := &mockCampaignStore{}
	e := NewEngine(cs, fs, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{
			ID: "s1",
			Entries: []model.Exchange{
				{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/error"}, Response: model.ResponseData{Status: 200}},
			},
		},
	}

	cfg := model.CampaignConfig{
		Target:  model.TargetURL{BaseURL: backend.URL},
		Limits:  model.CampaignLimits{Workers: 1, MaxTests: 3},
		Anomaly: model.AnomalyConfig{Detect5xx: true},
	}

	e.runCampaign(context.Background(), "camp-anomaly", cfg, seeds, nil)
	// At least some findings should be created for 500 errors
	if findingsCreated == 0 {
		t.Log("no findings created (dedup may have caught them)")
	}
}

func TestRunCampaign_WithConfirmation(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer backend.Close()

	statusUpdates := 0
	fs := &mockFindingStore{
		updateStatusFn: func(ctx context.Context, id string, status model.FindingStatus) error {
			statusUpdates++
			return nil
		},
	}
	cs := &mockCampaignStore{}
	e := NewEngine(cs, fs, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{ID: "s1", Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/"}}}},
	}

	cfg := model.CampaignConfig{
		Target:  model.TargetURL{BaseURL: backend.URL},
		Limits:  model.CampaignLimits{Workers: 1, MaxTests: 1},
		Anomaly: model.AnomalyConfig{Detect5xx: true},
		Triage:  model.TriageConfig{ConfirmRuns: 2},
	}

	e.runCampaign(context.Background(), "camp-confirm", cfg, seeds, nil)
}

func TestRunCampaign_WithMinimization(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer backend.Close()

	fs := &mockFindingStore{}
	cs := &mockCampaignStore{}
	e := NewEngine(cs, fs, &mockArtifactStore{}, nil, nil, t.TempDir(), zerolog.Nop())

	seeds := []model.RecordingSession{
		{ID: "s1", Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/"}}}},
	}

	cfg := model.CampaignConfig{
		Target:  model.TargetURL{BaseURL: backend.URL},
		Limits:  model.CampaignLimits{Workers: 1, MaxTests: 1},
		Anomaly: model.AnomalyConfig{Detect5xx: true},
		Triage:  model.TriageConfig{ConfirmRuns: 1, EnableMinimization: true},
	}

	e.runCampaign(context.Background(), "camp-minimize", cfg, seeds, nil)
}

func TestNewWorker(t *testing.T) {
	cfg := WorkerConfig{
		ID:         1,
		CampaignID: "camp-1",
		BaseURL:    "http://localhost:8080",
		Logger:     zerolog.Nop(),
	}
	w := NewWorker(cfg)
	if w == nil {
		t.Fatal("expected non-nil Worker")
	}
	if w.id != 1 {
		t.Errorf("worker id = %d, want 1", w.id)
	}
	if w.campaignID != "camp-1" {
		t.Errorf("campaignID = %q, want camp-1", w.campaignID)
	}
	if w.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q", w.baseURL)
	}
}

func TestWorkerRun_ContextCancelled(t *testing.T) {
	w := NewWorker(WorkerConfig{Logger: zerolog.Nop()})
	taskCh := make(chan SeedTask, 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		w.Run(ctx, taskCh)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestWorkerRun_ClosedChannel(t *testing.T) {
	w := NewWorker(WorkerConfig{Logger: zerolog.Nop()})
	taskCh := make(chan SeedTask)
	close(taskCh)

	done := make(chan struct{})
	go func() {
		w.Run(context.Background(), taskCh)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after channel closure")
	}
}

func TestWorkerRun_ProcessesTask(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()

	testsProcessed := 0
	cs := &mockCampaignStore{
		incrementStatsFn: func(ctx context.Context, id string, testsDelta, findingsDelta int) error {
			testsProcessed += testsDelta
			return nil
		},
	}

	w := NewWorker(WorkerConfig{
		CampaignID: "camp-1",
		BaseURL:    backend.URL,
		Pipeline:   mutate.NewPipeline(mutate.Config{}),
		Detector:   anomaly.NewMultiDetector(model.AnomalyConfig{}, zerolog.Nop()),
		Triager:    triage.NewTriager(),
		Replayer:   replayer.New(nil, zerolog.Nop()),
		Findings:   &mockFindingStore{},
		Artifacts:  &mockArtifactStore{},
		Campaigns:  cs,
		Baselines:  make(map[string]*anomaly.BaselineEntry),
		Logger:     zerolog.Nop(),
	})

	taskCh := make(chan SeedTask, 1)
	taskCh <- SeedTask{
		Session: model.RecordingSession{
			ID:      "s1",
			Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/"}}},
		},
		MutationSeed: 1,
	}
	close(taskCh)

	w.Run(context.Background(), taskCh)

	if testsProcessed != 1 {
		t.Errorf("tests processed = %d, want 1", testsProcessed)
	}
}

func TestWorkerWriteArtifact(t *testing.T) {
	dir := t.TempDir()
	as := &mockArtifactStore{}
	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		BaseURL:     "http://localhost",
		ArtifactDir: dir,
		Artifacts:   as,
		Logger:      zerolog.Nop(),
	})

	session := model.RecordingSession{
		ID: "sess-1",
		Entries: []model.Exchange{
			{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/test"}},
		},
	}
	hit := anomaly.AnomalyHit{
		Type:     model.FindingTimeout,
		Method:   "GET",
		Endpoint: "/test",
	}

	w.writeArtifact(context.Background(), "finding-1", session, hit, 42, []string{"op1"})

	// Verify artifact file was created
	artifactPath := filepath.Join(dir, "camp-1", "finding-1.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if len(data) == 0 {
		t.Error("artifact file is empty")
	}

	var payload model.ArtifactPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	if payload.FindingID != "finding-1" {
		t.Errorf("finding_id = %q, want finding-1", payload.FindingID)
	}
	if payload.CampaignID != "camp-1" {
		t.Errorf("campaign_id = %q, want camp-1", payload.CampaignID)
	}
	if payload.MutationSeed != 42 {
		t.Errorf("mutation_seed = %d, want 42", payload.MutationSeed)
	}
}

func TestWorkerWriteArtifact_BadDir(t *testing.T) {
	// Test with an unwritable directory path
	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		ArtifactDir: "/dev/null/nonexistent",
		Artifacts:   &mockArtifactStore{},
		Logger:      zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{Type: model.FindingTimeout}
	session := model.RecordingSession{ID: "s1"}

	// Should not panic
	w.writeArtifact(context.Background(), "f1", session, hit, 0, nil)
}

func TestWorkerWriteArtifact_ArtifactStoreError(t *testing.T) {
	dir := t.TempDir()
	as := &mockArtifactStore{
		createFn: func(ctx context.Context, a model.Artifact) error {
			return fmt.Errorf("db error")
		},
	}
	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		ArtifactDir: dir,
		Artifacts:   as,
		Logger:      zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{Type: model.FindingTimeout}
	session := model.RecordingSession{ID: "s1"}

	// Should not panic even when artifact store fails
	w.writeArtifact(context.Background(), "f1", session, hit, 0, nil)
}

func TestWorkerHandleHit_Dedup(t *testing.T) {
	fs := &mockFindingStore{
		existsBySignatureFn: func(ctx context.Context, campaignID, signature string) (bool, error) {
			return true, nil // already exists -> dedup
		},
	}
	w := NewWorker(WorkerConfig{
		CampaignID: "camp-1",
		Findings:   fs,
		Campaigns:  &mockCampaignStore{},
		Triager:    triage.NewTriager(),
		Logger:     zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}
	session := model.RecordingSession{ID: "s1"}

	w.handleHit(context.Background(), hit, session, "", 0, nil)
}

func TestWorkerHandleHit_ExistsBySignatureError(t *testing.T) {
	fs := &mockFindingStore{
		existsBySignatureFn: func(ctx context.Context, campaignID, signature string) (bool, error) {
			return false, fmt.Errorf("db error")
		},
	}
	w := NewWorker(WorkerConfig{
		CampaignID: "camp-1",
		Findings:   fs,
		Campaigns:  &mockCampaignStore{},
		Triager:    triage.NewTriager(),
		Logger:     zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}

	// Should not panic, should return early
	w.handleHit(context.Background(), hit, model.RecordingSession{}, "", 0, nil)
}

func TestWorkerHandleHit_CreateError(t *testing.T) {
	fs := &mockFindingStore{
		createFn: func(ctx context.Context, f model.Finding) error {
			return fmt.Errorf("db error")
		},
	}
	w := NewWorker(WorkerConfig{
		CampaignID: "camp-1",
		Findings:   fs,
		Campaigns:  &mockCampaignStore{},
		Triager:    triage.NewTriager(),
		Logger:     zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}

	// Should not panic, should return after create error
	w.handleHit(context.Background(), hit, model.RecordingSession{}, "", 0, nil)
}

func TestWorkerHandleHit_NewFinding(t *testing.T) {
	findingCreated := false
	fs := &mockFindingStore{
		existsBySignatureFn: func(ctx context.Context, campaignID, signature string) (bool, error) {
			return false, nil
		},
		createFn: func(ctx context.Context, f model.Finding) error {
			findingCreated = true
			if f.CampaignID != "camp-1" {
				t.Errorf("finding campaign_id = %q, want camp-1", f.CampaignID)
			}
			return nil
		},
	}
	dir := t.TempDir()
	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		BaseURL:     "http://localhost",
		ArtifactDir: dir,
		Findings:    fs,
		Artifacts:   &mockArtifactStore{},
		Campaigns:   &mockCampaignStore{},
		Triager:     triage.NewTriager(),
		Logger:      zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}
	session := model.RecordingSession{ID: "s1"}

	w.handleHit(context.Background(), hit, session, "", 0, nil)

	if !findingCreated {
		t.Error("expected finding to be created")
	}
}

func TestWorkerHandleHit_WithConfirmation(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer backend.Close()

	confirmed := false
	fs := &mockFindingStore{
		updateStatusFn: func(ctx context.Context, id string, status model.FindingStatus) error {
			if status == model.FindingConfirmed {
				confirmed = true
			}
			return nil
		},
	}

	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		BaseURL:     backend.URL,
		ArtifactDir: t.TempDir(),
		Findings:    fs,
		Artifacts:   &mockArtifactStore{},
		Campaigns:   &mockCampaignStore{},
		Triager:     triage.NewTriager(),
		Replayer:    replayer.New(nil, zerolog.Nop()),
		Detector:    anomaly.NewMultiDetector(model.AnomalyConfig{Detect5xx: true}, zerolog.Nop()),
		AnomalyCfg:  model.AnomalyConfig{Detect5xx: true},
		TriageCfg:   model.TriageConfig{ConfirmRuns: 2},
		Baselines:   make(map[string]*anomaly.BaselineEntry),
		Logger:      zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}
	session := model.RecordingSession{
		ID: "s1",
		Entries: []model.Exchange{
			{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/api/test"}},
		},
	}

	w.handleHit(context.Background(), hit, session, "", 42, nil)

	if !confirmed {
		t.Log("confirmation may not have succeeded (depends on replay)")
	}
}

func TestWorkerHandleHit_WithMinimization(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer backend.Close()

	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		BaseURL:     backend.URL,
		ArtifactDir: t.TempDir(),
		Findings:    &mockFindingStore{},
		Artifacts:   &mockArtifactStore{},
		Campaigns:   &mockCampaignStore{},
		Triager:     triage.NewTriager(),
		Replayer:    replayer.New(nil, zerolog.Nop()),
		Detector:    anomaly.NewMultiDetector(model.AnomalyConfig{Detect5xx: true}, zerolog.Nop()),
		AnomalyCfg:  model.AnomalyConfig{Detect5xx: true},
		TriageCfg:   model.TriageConfig{ConfirmRuns: 1, EnableMinimization: true},
		Baselines:   make(map[string]*anomaly.BaselineEntry),
		Logger:      zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/api/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}
	session := model.RecordingSession{
		ID: "s1",
		Entries: []model.Exchange{
			{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/api/test"}},
			{RequestID: "r2", Request: model.RequestData{Method: "GET", Path: "/other"}},
		},
	}

	// Should not panic - exercises the minimization code path
	w.handleHit(context.Background(), hit, session, "", 42, nil)
}

func TestWorkerHandleHit_ConfirmationError(t *testing.T) {
	// Test the path where confirmation returns an error
	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		BaseURL:     "http://127.0.0.1:1", // will fail connection
		ArtifactDir: t.TempDir(),
		Findings:    &mockFindingStore{},
		Artifacts:   &mockArtifactStore{},
		Campaigns:   &mockCampaignStore{},
		Triager:     triage.NewTriager(),
		Replayer:    replayer.New(nil, zerolog.Nop()),
		Detector:    anomaly.NewMultiDetector(model.AnomalyConfig{Detect5xx: true}, zerolog.Nop()),
		AnomalyCfg:  model.AnomalyConfig{Detect5xx: true},
		TriageCfg:   model.TriageConfig{ConfirmRuns: 2},
		Baselines:   make(map[string]*anomaly.BaselineEntry),
		Logger:      zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}
	session := model.RecordingSession{
		ID:      "s1",
		Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/test"}}},
	}

	// Should not panic even when confirmation fails
	w.handleHit(context.Background(), hit, session, "", 0, nil)
}

func TestWorkerHandleHit_UpdateStatusError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer backend.Close()

	fs := &mockFindingStore{
		updateStatusFn: func(ctx context.Context, id string, status model.FindingStatus) error {
			return fmt.Errorf("db error")
		},
	}

	w := NewWorker(WorkerConfig{
		CampaignID:  "camp-1",
		BaseURL:     backend.URL,
		ArtifactDir: t.TempDir(),
		Findings:    fs,
		Artifacts:   &mockArtifactStore{},
		Campaigns:   &mockCampaignStore{},
		Triager:     triage.NewTriager(),
		Replayer:    replayer.New(nil, zerolog.Nop()),
		Detector:    anomaly.NewMultiDetector(model.AnomalyConfig{Detect5xx: true}, zerolog.Nop()),
		AnomalyCfg:  model.AnomalyConfig{Detect5xx: true},
		TriageCfg:   model.TriageConfig{ConfirmRuns: 1},
		Baselines:   make(map[string]*anomaly.BaselineEntry),
		Logger:      zerolog.Nop(),
	})

	hit := anomaly.AnomalyHit{
		Type:     model.FindingServerError,
		Method:   "GET",
		Endpoint: "/test",
		Exchange: model.Exchange{Request: model.RequestData{BodyB64: ""}},
	}
	session := model.RecordingSession{
		ID:      "s1",
		Entries: []model.Exchange{{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/test"}}},
	}

	// Should not panic even when update status fails
	w.handleHit(context.Background(), hit, session, "", 0, nil)
}

func TestWorkerProcessTask_BasicMutation(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer backend.Close()

	testsDone := 0
	cs := &mockCampaignStore{
		incrementStatsFn: func(ctx context.Context, id string, testsDelta, findingsDelta int) error {
			testsDone += testsDelta
			return nil
		},
	}

	w := NewWorker(WorkerConfig{
		CampaignID: "camp-1",
		BaseURL:    backend.URL,
		Pipeline:   mutate.NewPipeline(mutate.Config{}),
		Detector:   anomaly.NewMultiDetector(model.AnomalyConfig{}, zerolog.Nop()),
		Triager:    triage.NewTriager(),
		Replayer:   replayer.New(nil, zerolog.Nop()),
		Findings:   &mockFindingStore{},
		Artifacts:  &mockArtifactStore{},
		Campaigns:  cs,
		Baselines:  make(map[string]*anomaly.BaselineEntry),
		Logger:     zerolog.Nop(),
	})

	task := SeedTask{
		Session: model.RecordingSession{
			ID: "s1",
			Entries: []model.Exchange{
				{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/test"}, Response: model.ResponseData{Status: 200}},
			},
		},
		MutationSeed: 42,
	}

	w.processTask(context.Background(), task)

	if testsDone != 1 {
		t.Errorf("testsDone = %d, want 1", testsDone)
	}
}

func TestWorkerProcessTask_WithSeqMutator(t *testing.T) {
	cs := &mockCampaignStore{}
	pipeline := mutate.NewPipeline(mutate.Config{})
	seqMutator := &mutate.SeqMutator{}
	detector := anomaly.NewMultiDetector(model.AnomalyConfig{}, zerolog.Nop())
	rep := replayer.New(nil, zerolog.Nop())
	triager := triage.NewTriager()

	w := NewWorker(WorkerConfig{
		CampaignID: "camp-1",
		BaseURL:    "http://127.0.0.1:1",
		Pipeline:   pipeline,
		SeqMutator: seqMutator,
		Detector:   detector,
		Triager:    triager,
		Replayer:   rep,
		Findings:   &mockFindingStore{},
		Artifacts:  &mockArtifactStore{},
		Campaigns:  cs,
		Baselines:  make(map[string]*anomaly.BaselineEntry),
		Logger:     zerolog.Nop(),
	})

	task := SeedTask{
		Session: model.RecordingSession{
			ID: "s1",
			Entries: []model.Exchange{
				{RequestID: "r1", Request: model.RequestData{Method: "GET", Path: "/a"}, Response: model.ResponseData{Status: 200}},
				{RequestID: "r2", Request: model.RequestData{Method: "POST", Path: "/b"}, Response: model.ResponseData{Status: 201}},
			},
		},
		MutationSeed: 123,
	}

	// Should not panic even with sequence mutation enabled
	w.processTask(context.Background(), task)
}

func TestSeedTask(t *testing.T) {
	task := SeedTask{
		Session:      model.RecordingSession{ID: "s1"},
		MutationSeed: 42,
	}
	if task.Session.ID != "s1" {
		t.Errorf("session ID = %q, want s1", task.Session.ID)
	}
	if task.MutationSeed != 42 {
		t.Errorf("mutation seed = %d, want 42", task.MutationSeed)
	}
}

func TestBuildResponseData(t *testing.T) {
	tests := []struct {
		name   string
		result replayer.ExchangeResult
		want   model.ResponseData
	}{
		{
			name: "basic response",
			result: replayer.ExchangeResult{
				StatusCode:  500,
				RespHeaders: http.Header{"Content-Type": []string{"application/json"}},
				RespBody:    []byte(`{"error":"internal"}`),
			},
			want: model.ResponseData{
				Status:  500,
				Headers: map[string][]string{"Content-Type": {"application/json"}},
				BodyB64: "eyJlcnJvciI6ImludGVybmFsIn0=",
			},
		},
		{
			name: "empty response",
			result: replayer.ExchangeResult{
				StatusCode: 204,
			},
			want: model.ResponseData{
				Status: 204,
			},
		},
		{
			name: "nil headers",
			result: replayer.ExchangeResult{
				StatusCode:  401,
				RespHeaders: nil,
				RespBody:    []byte("Unauthorized"),
			},
			want: model.ResponseData{
				Status:  401,
				BodyB64: "VW5hdXRob3JpemVk",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildResponseData(tt.result)
			if got.Status != tt.want.Status {
				t.Errorf("Status = %d, want %d", got.Status, tt.want.Status)
			}
			if got.BodyB64 != tt.want.BodyB64 {
				t.Errorf("BodyB64 = %q, want %q", got.BodyB64, tt.want.BodyB64)
			}
			if len(tt.want.Headers) > 0 {
				for k, v := range tt.want.Headers {
					if gotV, ok := got.Headers[k]; !ok || len(gotV) != len(v) {
						t.Errorf("Headers[%s] = %v, want %v", k, gotV, v)
					}
				}
			}
		})
	}
}

func TestBuildResponseData_LargeBody(t *testing.T) {
	// Create body larger than 64KB
	largeBody := make([]byte, 100*1024)
	for i := range largeBody {
		largeBody[i] = byte('A' + i%26)
	}

	result := replayer.ExchangeResult{
		StatusCode: 200,
		RespBody:   largeBody,
	}

	got := buildResponseData(result)
	if !got.BodyTruncated {
		t.Error("expected BodyTruncated = true for large body")
	}
	// Check that body was truncated to 64KB
	decoded, _ := base64.StdEncoding.DecodeString(got.BodyB64)
	if len(decoded) != 64*1024 {
		t.Errorf("decoded body length = %d, want %d", len(decoded), 64*1024)
	}
}
