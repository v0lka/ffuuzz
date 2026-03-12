package corpus

import (
	"context"
	"errors"
	"testing"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

type mockCampaignReader struct {
	campaign *model.Campaign
	err      error
}

func (m *mockCampaignReader) GetByID(_ context.Context, _ string) (*model.Campaign, error) {
	return m.campaign, m.err
}

type mockRecordingReader struct {
	sessions map[string]*model.RecordingSession
	err      error
}

func (m *mockRecordingReader) GetByID(_ context.Context, id string, _ bool, _ int) (*model.RecordingSession, error) {
	if m.err != nil {
		return nil, m.err
	}
	sess, ok := m.sessions[id]
	if !ok {
		return nil, nil
	}
	return sess, nil
}

func TestNewManager(t *testing.T) {
	m := NewManager(nil, nil, zerolog.Nop())
	if m == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestComputeBaseline_Single(t *testing.T) {
	sessions := []model.RecordingSession{
		{
			ID:     "sess-1",
			Target: model.TargetInfo{Path: "/api/users"},
			Entries: []model.Exchange{
				{
					DurationMs: 100,
					Request:    model.RequestData{Method: "GET", Path: "/api/users"},
					Response:   model.ResponseData{Status: 200},
				},
			},
		},
	}

	result := ComputeBaseline(sessions)
	entry, ok := result["GET|/api/users"]
	if !ok {
		t.Fatal("expected baseline entry for GET|/api/users")
	}
	if entry.P50Ms != 100 {
		t.Errorf("P50Ms = %d, want 100", entry.P50Ms)
	}
	if entry.Method != "GET" {
		t.Errorf("Method = %q, want GET", entry.Method)
	}
	if entry.Endpoint != "/api/users" {
		t.Errorf("Endpoint = %q, want /api/users", entry.Endpoint)
	}
	if entry.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", entry.StatusCode)
	}
}

func TestComputeBaseline_MultipleLatencies(t *testing.T) {
	sessions := []model.RecordingSession{
		{
			ID:     "sess-1",
			Target: model.TargetInfo{Path: "/api"},
			Entries: []model.Exchange{
				{DurationMs: 10, Request: model.RequestData{Method: "GET", Path: "/api"}, Response: model.ResponseData{Status: 200}},
				{DurationMs: 50, Request: model.RequestData{Method: "GET", Path: "/api"}, Response: model.ResponseData{Status: 200}},
				{DurationMs: 90, Request: model.RequestData{Method: "GET", Path: "/api"}, Response: model.ResponseData{Status: 200}},
			},
		},
	}

	result := ComputeBaseline(sessions)
	entry := result["GET|/api"]
	// sorted: [10, 50, 90], p50 = index 1 = 50
	if entry.P50Ms != 50 {
		t.Errorf("P50Ms = %d, want 50", entry.P50Ms)
	}
}

func TestComputeBaseline_MultipleSessions(t *testing.T) {
	sessions := []model.RecordingSession{
		{
			ID:     "sess-1",
			Target: model.TargetInfo{Path: "/a"},
			Entries: []model.Exchange{
				{DurationMs: 100, Request: model.RequestData{Method: "GET", Path: "/a"}, Response: model.ResponseData{Status: 200}},
			},
		},
		{
			ID:     "sess-2",
			Target: model.TargetInfo{Path: "/a"},
			Entries: []model.Exchange{
				{DurationMs: 200, Request: model.RequestData{Method: "GET", Path: "/a"}, Response: model.ResponseData{Status: 200}},
			},
		},
	}

	result := ComputeBaseline(sessions)
	entry := result["GET|/a"]
	// sorted: [100, 200], p50 = (100+200)/2 = 150
	if entry.P50Ms != 150 {
		t.Errorf("P50Ms = %d, want 150", entry.P50Ms)
	}
}

func TestComputeBaseline_MultipleEndpoints(t *testing.T) {
	sessions := []model.RecordingSession{
		{
			ID:     "sess-1",
			Target: model.TargetInfo{Path: "/a"},
			Entries: []model.Exchange{
				{DurationMs: 100, Request: model.RequestData{Method: "GET", Path: "/a"}, Response: model.ResponseData{Status: 200}},
			},
		},
		{
			ID:     "sess-2",
			Target: model.TargetInfo{Path: "/b"},
			Entries: []model.Exchange{
				{DurationMs: 200, Request: model.RequestData{Method: "POST", Path: "/b"}, Response: model.ResponseData{Status: 201}},
			},
		},
	}

	result := ComputeBaseline(sessions)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	if _, ok := result["GET|/a"]; !ok {
		t.Error("missing GET|/a")
	}
	if _, ok := result["POST|/b"]; !ok {
		t.Error("missing POST|/b")
	}
}

func TestComputeBaseline_Empty(t *testing.T) {
	result := ComputeBaseline(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(result))
	}
}

func TestComputeBaseline_EmptySessions(t *testing.T) {
	result := ComputeBaseline([]model.RecordingSession{})
	if len(result) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(result))
	}
}

func TestGetSeeds_Success(t *testing.T) {
	sess1 := &model.RecordingSession{ID: "rec-1", Entries: []model.Exchange{{RequestID: "r1"}}}
	sess2 := &model.RecordingSession{ID: "rec-2", Entries: []model.Exchange{{RequestID: "r2"}}}

	m := NewManager(
		&mockRecordingReader{sessions: map[string]*model.RecordingSession{"rec-1": sess1, "rec-2": sess2}},
		&mockCampaignReader{campaign: &model.Campaign{ID: "camp-1", RecordingIDs: []string{"rec-1", "rec-2"}}},
		zerolog.Nop(),
	)

	seeds, err := m.GetSeeds(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 2 {
		t.Fatalf("expected 2 seeds, got %d", len(seeds))
	}
	if seeds[0].ID != "rec-1" {
		t.Errorf("seeds[0].ID = %q, want rec-1", seeds[0].ID)
	}
	if seeds[1].ID != "rec-2" {
		t.Errorf("seeds[1].ID = %q, want rec-2", seeds[1].ID)
	}
}

func TestGetSeeds_CampaignNotFound(t *testing.T) {
	m := NewManager(
		&mockRecordingReader{sessions: nil},
		&mockCampaignReader{campaign: nil},
		zerolog.Nop(),
	)

	_, err := m.GetSeeds(context.Background(), "missing-camp")
	if err == nil {
		t.Fatal("expected error for missing campaign")
	}
}

func TestGetSeeds_CampaignGetError(t *testing.T) {
	m := NewManager(
		&mockRecordingReader{sessions: nil},
		&mockCampaignReader{err: errors.New("db connection failed")},
		zerolog.Nop(),
	)

	_, err := m.GetSeeds(context.Background(), "camp-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetSeeds_RecordingGetError(t *testing.T) {
	m := NewManager(
		&mockRecordingReader{err: errors.New("recording fetch failed")},
		&mockCampaignReader{campaign: &model.Campaign{ID: "camp-1", RecordingIDs: []string{"rec-1"}}},
		zerolog.Nop(),
	)

	_, err := m.GetSeeds(context.Background(), "camp-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetSeeds_RecordingNotFound_Skipped(t *testing.T) {
	sess1 := &model.RecordingSession{ID: "rec-1", Entries: []model.Exchange{{RequestID: "r1"}}}

	m := NewManager(
		&mockRecordingReader{sessions: map[string]*model.RecordingSession{"rec-1": sess1}},
		&mockCampaignReader{campaign: &model.Campaign{ID: "camp-1", RecordingIDs: []string{"rec-1", "rec-missing"}}},
		zerolog.Nop(),
	)

	seeds, err := m.GetSeeds(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 1 {
		t.Fatalf("expected 1 seed (missing recording skipped), got %d", len(seeds))
	}
	if seeds[0].ID != "rec-1" {
		t.Errorf("seeds[0].ID = %q, want rec-1", seeds[0].ID)
	}
}

func TestGetSeeds_NoRecordings(t *testing.T) {
	m := NewManager(
		&mockRecordingReader{sessions: nil},
		&mockCampaignReader{campaign: &model.Campaign{ID: "camp-1", RecordingIDs: nil}},
		zerolog.Nop(),
	)

	seeds, err := m.GetSeeds(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(seeds) != 0 {
		t.Fatalf("expected 0 seeds, got %d", len(seeds))
	}
}
