package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

// mockReproduceFindingStore extends mockFindingStore with reproduce-specific behavior.
type mockReproduceFindingStore struct {
	getByIDFn               func(ctx context.Context, id string) (*model.Finding, error)
	claimNextReproduceJobFn func(ctx context.Context) (string, int, bool, error)
	setReproduceStatusFn    func(ctx context.Context, id, status string) error
	existsBySignatureFn     func(ctx context.Context, campaignID, signature string) (bool, error)
	createFn                func(ctx context.Context, f model.Finding) error
	updateStatusFn          func(ctx context.Context, id string, status model.FindingStatus) error
}

func (m *mockReproduceFindingStore) GetByID(ctx context.Context, id string) (*model.Finding, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockReproduceFindingStore) ClaimNextReproduceJob(ctx context.Context) (string, int, bool, error) {
	if m.claimNextReproduceJobFn != nil {
		return m.claimNextReproduceJobFn(ctx)
	}
	return "", 0, false, nil
}
func (m *mockReproduceFindingStore) SetReproduceStatus(ctx context.Context, id, status string) error {
	if m.setReproduceStatusFn != nil {
		return m.setReproduceStatusFn(ctx, id, status)
	}
	return nil
}
func (m *mockReproduceFindingStore) ExistsBySignature(ctx context.Context, campaignID, signature string) (bool, error) {
	if m.existsBySignatureFn != nil {
		return m.existsBySignatureFn(ctx, campaignID, signature)
	}
	return false, nil
}
func (m *mockReproduceFindingStore) Create(ctx context.Context, f model.Finding) error {
	if m.createFn != nil {
		return m.createFn(ctx, f)
	}
	return nil
}
func (m *mockReproduceFindingStore) UpdateStatus(ctx context.Context, id string, status model.FindingStatus) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status)
	}
	return nil
}

// mockReproduceArtifactStore extends mockArtifactStore with GetByFindingID.
type mockReproduceArtifactStore struct {
	createFn         func(ctx context.Context, a model.Artifact) error
	getByFindingIDFn func(ctx context.Context, findingID string) (*model.Artifact, error)
}

func (m *mockReproduceArtifactStore) Create(ctx context.Context, a model.Artifact) error {
	if m.createFn != nil {
		return m.createFn(ctx, a)
	}
	return nil
}
func (m *mockReproduceArtifactStore) GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error) {
	if m.getByFindingIDFn != nil {
		return m.getByFindingIDFn(ctx, findingID)
	}
	return nil, nil
}

func TestNewReproduceWorker(t *testing.T) {
	fs := &mockReproduceFindingStore{}
	as := &mockReproduceArtifactStore{}
	w := NewReproduceWorker(fs, as, "/tmp/artifacts", zerolog.Nop())
	if w == nil {
		t.Fatal("expected non-nil ReproduceWorker")
	}
	if w.findings != fs {
		t.Error("findings store not set")
	}
	if w.artifacts != as {
		t.Error("artifacts store not set")
	}
	if w.artifactDir != "/tmp/artifacts" {
		t.Errorf("artifactDir = %q", w.artifactDir)
	}
}

func TestReproduceWorker_Run_ContextCancel(t *testing.T) {
	w := NewReproduceWorker(
		&mockReproduceFindingStore{},
		&mockReproduceArtifactStore{},
		"/tmp",
		zerolog.Nop(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after context cancellation")
	}
}

func TestReproduceWorker_Poll_NoJob(t *testing.T) {
	fs := &mockReproduceFindingStore{
		claimNextReproduceJobFn: func(ctx context.Context) (string, int, bool, error) {
			return "", 0, false, nil
		},
	}
	w := NewReproduceWorker(fs, &mockReproduceArtifactStore{}, "/tmp", zerolog.Nop())
	// poll should return without error when no job is found
	w.poll(context.Background())
}

func TestReproduceWorker_Poll_ClaimError(t *testing.T) {
	fs := &mockReproduceFindingStore{
		claimNextReproduceJobFn: func(ctx context.Context) (string, int, bool, error) {
			return "", 0, false, errors.New("db error")
		},
	}
	w := NewReproduceWorker(fs, &mockReproduceArtifactStore{}, "/tmp", zerolog.Nop())
	// Should not panic
	w.poll(context.Background())
}

func TestReproduceWorker_FailJob(t *testing.T) {
	var statusSet string
	fs := &mockReproduceFindingStore{
		setReproduceStatusFn: func(ctx context.Context, id, status string) error {
			statusSet = status
			return nil
		},
	}
	w := NewReproduceWorker(fs, &mockReproduceArtifactStore{}, "/tmp", zerolog.Nop())
	w.failJob(context.Background(), "finding-1")
	if statusSet != "FAILED" {
		t.Errorf("status = %q, want FAILED", statusSet)
	}
}

func TestReproduceWorker_ProcessJob_FindingNotFound(t *testing.T) {
	var statusSet string
	fs := &mockReproduceFindingStore{
		getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
			return nil, nil
		},
		setReproduceStatusFn: func(ctx context.Context, id, status string) error {
			statusSet = status
			return nil
		},
	}
	w := NewReproduceWorker(fs, &mockReproduceArtifactStore{}, "/tmp", zerolog.Nop())
	w.processJob(context.Background(), "finding-1", 3)
	if statusSet != "FAILED" {
		t.Errorf("status = %q, want FAILED", statusSet)
	}
}

func TestReproduceWorker_ProcessJob_ArtifactNotFound(t *testing.T) {
	var statusSet string
	fs := &mockReproduceFindingStore{
		getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
			return &model.Finding{ID: id, Type: model.FindingServerError}, nil
		},
		setReproduceStatusFn: func(ctx context.Context, id, status string) error {
			statusSet = status
			return nil
		},
	}
	as := &mockReproduceArtifactStore{
		getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
			return nil, nil
		},
	}
	w := NewReproduceWorker(fs, as, "/tmp", zerolog.Nop())
	w.processJob(context.Background(), "finding-1", 3)
	if statusSet != "FAILED" {
		t.Errorf("status = %q, want FAILED", statusSet)
	}
}

func TestReproduceWorker_ProcessJob_ArtifactFileNotFound(t *testing.T) {
	var statusSet string
	fs := &mockReproduceFindingStore{
		getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
			return &model.Finding{ID: id, Type: model.FindingServerError}, nil
		},
		setReproduceStatusFn: func(ctx context.Context, id, status string) error {
			statusSet = status
			return nil
		},
	}
	as := &mockReproduceArtifactStore{
		getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
			return &model.Artifact{ID: "art-1", FilePath: "nonexistent/file.json"}, nil
		},
	}
	w := NewReproduceWorker(fs, as, "/tmp/nonexistent_dir", zerolog.Nop())
	w.processJob(context.Background(), "finding-1", 3)
	if statusSet != "FAILED" {
		t.Errorf("status = %q, want FAILED", statusSet)
	}
}

func TestReproduceWorker_ProcessJob_InvalidArtifactJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Write invalid JSON to artifact file
	artPath := filepath.Join(tmpDir, "art.json")
	if err := os.WriteFile(artPath, []byte("not json"), 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var statusSet string
	fs := &mockReproduceFindingStore{
		getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
			return &model.Finding{ID: id, Type: model.FindingServerError}, nil
		},
		setReproduceStatusFn: func(ctx context.Context, id, status string) error {
			statusSet = status
			return nil
		},
	}
	as := &mockReproduceArtifactStore{
		getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
			return &model.Artifact{ID: "art-1", FilePath: "art.json"}, nil
		},
	}
	w := NewReproduceWorker(fs, as, tmpDir, zerolog.Nop())
	w.processJob(context.Background(), "finding-1", 3)
	if statusSet != "FAILED" {
		t.Errorf("status = %q, want FAILED", statusSet)
	}
}

func TestReproduceWorker_ProcessJob_DefaultRuns(t *testing.T) {
	tmpDir := t.TempDir()

	payload := model.ArtifactPayload{
		Target:  model.TargetURL{BaseURL: "http://127.0.0.1:1"}, // unreachable
		Session: model.RecordingSession{Entries: []model.Exchange{{}}},
	}
	data, _ := json.Marshal(payload)
	artDir := filepath.Join(tmpDir, "campaign-1")
	_ = os.MkdirAll(artDir, 0755)
	artPath := filepath.Join(artDir, "finding-1.json")
	if err := os.WriteFile(artPath, data, 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var finalStatus string
	fs := &mockReproduceFindingStore{
		getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
			return &model.Finding{ID: id, Type: model.FindingServerError}, nil
		},
		setReproduceStatusFn: func(ctx context.Context, id, status string) error {
			finalStatus = status
			return nil
		},
	}
	as := &mockReproduceArtifactStore{
		getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
			return &model.Artifact{ID: "art-1", FilePath: "campaign-1/finding-1.json"}, nil
		},
	}

	w := NewReproduceWorker(fs, as, tmpDir, zerolog.Nop())
	// runs=0 should default to 3
	w.processJob(context.Background(), "finding-1", 0)

	// Since target is unreachable, replay will fail, result should be NOT_REPRODUCED
	if finalStatus != "NOT_REPRODUCED" {
		t.Errorf("status = %q, want NOT_REPRODUCED", finalStatus)
	}
}

func TestReproduceWorker_ProcessJob_ContextCancelled(t *testing.T) {
	tmpDir := t.TempDir()

	payload := model.ArtifactPayload{
		Target:  model.TargetURL{BaseURL: "http://127.0.0.1:1"},
		Session: model.RecordingSession{Entries: []model.Exchange{{}}},
	}
	data, _ := json.Marshal(payload)
	artDir := filepath.Join(tmpDir, "campaign-1")
	_ = os.MkdirAll(artDir, 0755)
	if err := os.WriteFile(filepath.Join(artDir, "finding-1.json"), data, 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}

	var finalStatus string
	fs := &mockReproduceFindingStore{
		getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
			return &model.Finding{ID: id, Type: model.FindingTimeout}, nil
		},
		setReproduceStatusFn: func(ctx context.Context, id, status string) error {
			finalStatus = status
			return nil
		},
	}
	as := &mockReproduceArtifactStore{
		getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
			return &model.Artifact{ID: "art-1", FilePath: "campaign-1/finding-1.json"}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	w := NewReproduceWorker(fs, as, tmpDir, zerolog.Nop())
	w.processJob(ctx, "finding-1", 3)
	if finalStatus != "FAILED" {
		t.Errorf("status = %q, want FAILED (cancelled context)", finalStatus)
	}
}

func TestStartReproduceWorker(t *testing.T) {
	e := NewEngine(
		&mockCampaignStore{},
		&mockReproduceFindingStore{},
		&mockReproduceArtifactStore{},
		nil,
		"/tmp",
		zerolog.Nop(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	e.StartReproduceWorker(ctx)

	// reproduceCancel should be set
	e.mu.Lock()
	hasCancel := e.reproduceCancel != nil
	e.mu.Unlock()

	if !hasCancel {
		t.Error("expected reproduceCancel to be set")
	}

	cancel()
	e.reproduceWg.Wait()
}

func TestStopAll_WithReproduceWorker(t *testing.T) {
	e := NewEngine(
		&mockCampaignStore{},
		&mockReproduceFindingStore{},
		&mockReproduceArtifactStore{},
		nil,
		"/tmp",
		zerolog.Nop(),
	)

	ctx := context.Background()
	e.StartReproduceWorker(ctx)

	// StopAll should stop the reproduce worker
	e.StopAll(ctx)
}

func TestStopAll_NoReproduceWorker(t *testing.T) {
	e := NewEngine(
		&mockCampaignStore{},
		&mockReproduceFindingStore{},
		&mockReproduceArtifactStore{},
		nil,
		"/tmp",
		zerolog.Nop(),
	)

	// Should not panic when reproduceCancel is nil
	e.StopAll(context.Background())
}
