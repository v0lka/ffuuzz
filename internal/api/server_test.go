package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

type mockRecordingStore struct {
	getByIDFn                func(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error)
	getByIDsFn               func(ctx context.Context, ids []string) ([]model.RecordingSession, error)
	upsertFn                 func(ctx context.Context, sess model.RecordingSession) (bool, error)
	listFn                   func(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error)
	listAllFn                func(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error)
	deleteFn                 func(ctx context.Context, id string) (bool, error)
	isUsedByActiveCampaignFn func(ctx context.Context, id string) (bool, error)
	getTreeFn                func(ctx context.Context) ([]model.TreeEntry, error)
	deleteByPrefixFn         func(ctx context.Context, scheme, host string, port int, pathPrefix string) (int64, error)
}

func (m *mockRecordingStore) GetByID(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id, includeEntries, maxBodyBytes)
	}
	return nil, nil
}
func (m *mockRecordingStore) GetByIDs(ctx context.Context, ids []string) ([]model.RecordingSession, error) {
	if m.getByIDsFn != nil {
		return m.getByIDsFn(ctx, ids)
	}
	return nil, nil
}
func (m *mockRecordingStore) Upsert(ctx context.Context, sess model.RecordingSession) (bool, error) {
	if m.upsertFn != nil {
		return m.upsertFn(ctx, sess)
	}
	return true, nil
}
func (m *mockRecordingStore) List(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
	if m.listFn != nil {
		return m.listFn(ctx, limit, offset, hostFilter, pathPrefix)
	}
	return nil, nil
}
func (m *mockRecordingStore) Delete(ctx context.Context, id string) (bool, error) {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return true, nil
}
func (m *mockRecordingStore) IsUsedByActiveCampaign(ctx context.Context, id string) (bool, error) {
	if m.isUsedByActiveCampaignFn != nil {
		return m.isUsedByActiveCampaignFn(ctx, id)
	}
	return false, nil
}
func (m *mockRecordingStore) GetTree(ctx context.Context) ([]model.TreeEntry, error) {
	if m.getTreeFn != nil {
		return m.getTreeFn(ctx)
	}
	return nil, nil
}
func (m *mockRecordingStore) DeleteByPrefix(ctx context.Context, scheme, host string, port int, pathPrefix string) (int64, error) {
	if m.deleteByPrefixFn != nil {
		return m.deleteByPrefixFn(ctx, scheme, host, port, pathPrefix)
	}
	return 0, nil
}
func (m *mockRecordingStore) ListAll(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx, hostFilter, pathPrefix)
	}
	return nil, nil
}

type mockCampaignStore struct {
	getByIDFn               func(ctx context.Context, id string) (*model.Campaign, error)
	createFn                func(ctx context.Context, c model.Campaign) error
	listFn                  func(ctx context.Context, statusFilter string, limit, offset int) ([]model.Campaign, error)
	addRecordingsByFilterFn func(ctx context.Context, campaignID, scheme, host string, port int, pathPrefix string) (int, error)
}

func (m *mockCampaignStore) GetByID(ctx context.Context, id string) (*model.Campaign, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockCampaignStore) Create(ctx context.Context, c model.Campaign) error {
	if m.createFn != nil {
		return m.createFn(ctx, c)
	}
	return nil
}
func (m *mockCampaignStore) List(ctx context.Context, statusFilter string, limit, offset int) ([]model.Campaign, error) {
	if m.listFn != nil {
		return m.listFn(ctx, statusFilter, limit, offset)
	}
	return nil, nil
}
func (m *mockCampaignStore) AddRecordingsByFilter(ctx context.Context, campaignID, scheme, host string, port int, pathPrefix string) (int, error) {
	if m.addRecordingsByFilterFn != nil {
		return m.addRecordingsByFilterFn(ctx, campaignID, scheme, host, port, pathPrefix)
	}
	return 0, nil
}

type mockFindingStore struct {
	listAllFn               func(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error)
	getByIDFn               func(ctx context.Context, id string) (*model.Finding, error)
	updateReproduceStatusFn func(ctx context.Context, id, status string, runs int) error
	countByTypeFn           func(ctx context.Context, campaignID string) (map[model.FindingType]int, error)
}

func (m *mockFindingStore) ListAll(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error) {
	if m.listAllFn != nil {
		return m.listAllFn(ctx, campaignID, typeFilter, statusFilter, since, limit, offset)
	}
	return nil, nil
}
func (m *mockFindingStore) GetByID(ctx context.Context, id string) (*model.Finding, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, nil
}
func (m *mockFindingStore) UpdateReproduceStatus(ctx context.Context, id, status string, runs int) error {
	if m.updateReproduceStatusFn != nil {
		return m.updateReproduceStatusFn(ctx, id, status, runs)
	}
	return nil
}
func (m *mockFindingStore) CountByType(ctx context.Context, campaignID string) (map[model.FindingType]int, error) {
	if m.countByTypeFn != nil {
		return m.countByTypeFn(ctx, campaignID)
	}
	return nil, nil
}

type mockArtifactStore struct {
	getByFindingIDFn func(ctx context.Context, findingID string) (*model.Artifact, error)
}

func (m *mockArtifactStore) GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error) {
	if m.getByFindingIDFn != nil {
		return m.getByFindingIDFn(ctx, findingID)
	}
	return nil, nil
}

type mockHealthChecker struct {
	pingFn func(ctx context.Context) error
}

func (m *mockHealthChecker) Ping(ctx context.Context) error {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return nil
}

func newTestServer(opts ...func(*ServerConfig)) *Server {
	cfg := ServerConfig{
		Addr:       ":0",
		Recordings: &mockRecordingStore{},
		Campaigns:  &mockCampaignStore{},
		Findings:   &mockFindingStore{},
		Artifacts:  &mockArtifactStore{},
		Health:     &mockHealthChecker{},
		Logger:     zerolog.Nop(),
	}
	for _, o := range opts {
		o(&cfg)
	}
	return NewServer(cfg)
}

func TestHealthz_OK(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v", body["status"])
	}
}

func TestHealthz_DBError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Health = &mockHealthChecker{pingFn: func(ctx context.Context) error {
			return context.DeadlineExceeded
		}}
	})
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestListRecordings_Empty(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/recordings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestListRecordings_WithData(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listFn: func(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				return []model.RecordingSession{{ID: "sess-1"}}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetRecording_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/recordings/99999999-9999-9999-9999-999999999999", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetRecording_Found(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error) {
				return &model.RecordingSession{ID: id}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestDeleteRecording_Success(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestDeleteRecording_InUse(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			isUsedByActiveCampaignFn: func(ctx context.Context, id string) (bool, error) {
				return true, nil
			},
		}
	})
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestDeleteRecording_NotFound(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			deleteFn: func(ctx context.Context, id string) (bool, error) {
				return false, nil
			},
		}
	})
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/99999999-9999-9999-9999-999999999999", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestImportRecordings_Success(t *testing.T) {
	srv := newTestServer()
	body := `{"sessions":[{"id":"s1","entries":[]}]}`
	req := httptest.NewRequest("POST", "/api/v1/recordings/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestImportRecordings_EmptySessions(t *testing.T) {
	srv := newTestServer()
	body := `{"sessions":[]}`
	req := httptest.NewRequest("POST", "/api/v1/recordings/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestImportRecordings_InvalidBody(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/api/v1/recordings/import", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestImportRecordings_DuplicateIDs(t *testing.T) {
	srv := newTestServer()
	body := `{"sessions":[{"id":"s1"},{"id":"s1"}]}`
	req := httptest.NewRequest("POST", "/api/v1/recordings/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestImportRecordings_WrongContentType(t *testing.T) {
	srv := newTestServer()
	body := `{"sessions":[{"id":"s1"}]}`
	req := httptest.NewRequest("POST", "/api/v1/recordings/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 415 {
		t.Errorf("status = %d, want 415", w.Code)
	}
}

func TestImportRecordings_Skipped(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			upsertFn: func(ctx context.Context, sess model.RecordingSession) (bool, error) {
				return false, nil // not inserted -> skipped
			},
		}
	})
	body := `{"sessions":[{"id":"s1"}]}`
	req := httptest.NewRequest("POST", "/api/v1/recordings/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Errorf("status = %d, want 201", w.Code)
	}
	var result importResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
}

func TestListCampaigns_Empty(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestListCampaigns_WithData(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			listFn: func(ctx context.Context, statusFilter string, limit, offset int) ([]model.Campaign, error) {
				return []model.Campaign{{ID: "c1"}}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetCampaign_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/campaigns/99999999-9999-9999-9999-999999999999", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetCampaign_Found(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{ID: id, Status: model.CampaignCreated}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestCreateCampaign_Success(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, _ bool, _ int) (*model.RecordingSession, error) {
				return &model.RecordingSession{ID: id}, nil
			},
		}
	})
	body := validCampaignBody()
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Errorf("status = %d, want 201, body = %s", w.Code, w.Body.String())
	}
}

func TestCreateCampaign_InvalidBody(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader("bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateCampaign_NoRecordings(t *testing.T) {
	srv := newTestServer()
	body := `{"name":"test","recording_ids":[],"config":{"target":{"base_url":"http://localhost"}}}`
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateCampaign_RecordingNotFound(t *testing.T) {
	srv := newTestServer()
	body := `{"name":"test","recording_ids":["r1"],"config":{"target":{"base_url":"http://localhost"}}}`
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetCampaignStats_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetCampaignStats_Found(t *testing.T) {
	now := time.Now()
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{
					ID:           id,
					Status:       model.CampaignRunning,
					StartedAt:    &now,
					TestsDone:    100,
					RecordingIDs: []string{"r1"},
				}, nil
			},
		}
		cfg.Findings = &mockFindingStore{
			countByTypeFn: func(ctx context.Context, campaignID string) (map[model.FindingType]int, error) {
				return map[model.FindingType]int{
					model.FindingTimeout:     1,
					model.FindingServerError: 1,
				}, nil
			},
		}
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, _ bool, _ int) (*model.RecordingSession, error) {
				return &model.RecordingSession{ID: id, EntryCount: 5}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetCampaignConfig_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetCampaignConfig_Found(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{
					ID:     id,
					Config: model.CampaignConfig{Target: model.TargetURL{BaseURL: "http://localhost"}},
				}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetCampaignFindings_Empty(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = campaignExistsStore()
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/findings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestGetCampaignFindings_WithData(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = campaignExistsStore()
		cfg.Findings = &mockFindingStore{
			listAllFn: func(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error) {
				return []model.Finding{{ID: "f1"}}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/findings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestListFindings_Empty(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestListFindings_WithData(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			listAllFn: func(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error) {
				return []model.Finding{{ID: "f1"}}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/findings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestListFindings_WithSinceParam(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings?since=2024-01-01T00:00:00Z", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	// Should still work even with since param (just returns 204 because no data)
	if w.Code != 204 {
		t.Errorf("status = %d, want 204", w.Code)
	}
}

func TestListFindings_WithInvalidSince(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings?since=not-a-date", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_SINCE") {
		t.Errorf("response body does not contain INVALID_SINCE error code: %s", body)
	}
}

func TestGetFinding_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetFinding_Found(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return &model.Finding{ID: id}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetFindingArtifact_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333/artifact", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestGetFindingArtifact_FileReadError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Artifacts = &mockArtifactStore{
			getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
				return &model.Artifact{ID: "a1", FilePath: "nonexistent.json"}, nil
			},
		}
		cfg.ArtifactDir = "/tmp/does-not-exist-ffuuzz"
	})
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333/artifact", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestReproduceFinding_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`{"runs":3}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestReproduceFinding_InvalidRuns(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return &model.Finding{ID: id}, nil
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`{"runs":50}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 422 {
		t.Errorf("status = %d, want 422", w.Code)
	}
}

func TestReproduceFinding_AlreadyQueued(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return &model.Finding{ID: id, ReproduceStatus: "ENQUEUED"}, nil
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`{"runs":3}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestReproduceFinding_Success(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return &model.Finding{ID: id}, nil
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`{"runs":3}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Errorf("status = %d, want 202", w.Code)
	}
}

func TestReproduceFinding_DefaultRuns(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return &model.Finding{ID: id}, nil
			},
		}
	})
	// Send body that fails binding so default runs=3 is used
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 202 {
		t.Errorf("status = %d, want 202", w.Code)
	}
}

func TestSpaHandler_NoWebFS(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/ui/", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestSpaHandler_WithWebFS(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<html></html>")},
		"assets/app.js":    &fstest.MapFile{Data: []byte("console.log('hi')")},
		"assets/style.css": &fstest.MapFile{Data: []byte("body{}")},
	}
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.WebFS = fs.FS(webFS)
	})

	// Root should serve index.html
	req := httptest.NewRequest("GET", "/ui/", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200 for /ui/", w.Code)
	}

	// Asset should be served
	req = httptest.NewRequest("GET", "/ui/assets/app.js", nil)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200 for asset", w.Code)
	}
	if !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Error("expected long cache for asset")
	}

	// Unknown path should fallback to index.html (SPA routing)
	req = httptest.NewRequest("GET", "/ui/some/page", nil)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200 for SPA fallback", w.Code)
	}
}

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status model.CampaignStatus
		want   bool
	}{
		{model.CampaignFinished, true},
		{model.CampaignFailed, true},
		{model.CampaignStopped, true},
		{model.CampaignRunning, false},
		{model.CampaignCreated, false},
		{model.CampaignStarting, false},
	}
	for _, tt := range tests {
		got := isTerminalStatus(tt.status)
		if got != tt.want {
			t.Errorf("isTerminalStatus(%q) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestContentTypeByExt(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"index.html", "text/html; charset=utf-8"},
		{"app.js", "application/javascript"},
		{"style.css", "text/css"},
		{"data.json", "application/json"},
		{"logo.svg", "image/svg+xml"},
		{"logo.png", "image/png"},
		{"favicon.ico", "image/x-icon"},
		{"font.woff2", "font/woff2"},
		{"font.woff", "font/woff"},
		{"font.ttf", "font/ttf"},
		{"app.js.map", "application/json"},
		{"unknown.xyz", "application/octet-stream"},
		{"app.mjs", "application/javascript"},
	}
	for _, tt := range tests {
		got := contentTypeByExt(tt.name)
		if got != tt.want {
			t.Errorf("contentTypeByExt(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestRequestIDMiddleware_Generates(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header")
	}
}

func TestRequestIDMiddleware_PreservesExisting(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/healthz", nil)
	req.Header.Set("X-Request-ID", "custom-id-123")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Header().Get("X-Request-ID") != "custom-id-123" {
		t.Errorf("X-Request-ID = %q, want custom-id-123", w.Header().Get("X-Request-ID"))
	}
}

func TestRootRedirect(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 302 {
		t.Errorf("status = %d, want 302", w.Code)
	}
}

func TestNewServer(t *testing.T) {
	srv := newTestServer()
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
}

// validCampaignBody returns a JSON body for creating a campaign with valid config.
func validCampaignBody() string {
	return `{"name":"test","recording_ids":["r1"],"config":{"target":{"base_url":"http://localhost"},"limits":{"workers":2,"rps":10,"req_timeout_ms":5000,"max_tests":100},"mutations":{"intensity":0.5}}}`
}

// campaignExistsStore returns a mockCampaignStore where GetByID always returns a campaign.
func campaignExistsStore() *mockCampaignStore {
	return &mockCampaignStore{
		getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
			return &model.Campaign{ID: id, Status: model.CampaignCreated}, nil
		},
	}
}

func TestStartCampaign_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/start", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStartCampaign_AlreadyRunning(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{ID: id, Status: model.CampaignRunning}, nil
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/start", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestStartCampaign_InvalidState(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{ID: id, Status: model.CampaignStopping}, nil
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/start", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 422 {
		t.Errorf("status = %d, want 422", w.Code)
	}
}

func TestStopCampaign_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stop", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStopCampaign_NotRunning(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{ID: id, Status: model.CampaignCreated}, nil
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stop", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestStreamCampaignStats_NotFound(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stream", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestStreamCampaignStats_TerminalStatus(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{
					ID:     id,
					Status: model.CampaignFinished,
				}, nil
			},
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stream", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Errorf("expected SSE done event, got: %s", body)
	}
}

func TestDeleteRecording_CheckError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			isUsedByActiveCampaignFn: func(ctx context.Context, id string) (bool, error) {
				return false, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestImportRecordings_UpsertError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			upsertFn: func(ctx context.Context, sess model.RecordingSession) (bool, error) {
				return false, context.DeadlineExceeded
			},
		}
	})
	body := `{"sessions":[{"id":"s1"}]}`
	req := httptest.NewRequest("POST", "/api/v1/recordings/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Errorf("status = %d, want 201 (partial result)", w.Code)
	}
	var result importResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
}

func TestListRecordings_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listFn: func(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetRecording_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestListCampaigns_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			listFn: func(ctx context.Context, statusFilter string, limit, offset int) ([]model.Campaign, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetCampaign_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetCampaignFindings_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = campaignExistsStore()
		cfg.Findings = &mockFindingStore{
			listAllFn: func(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/findings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestListFindings_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			listAllFn: func(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/findings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetFinding_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetFindingArtifact_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Artifacts = &mockArtifactStore{
			getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333/artifact", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestReproduceFinding_UpdateError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return &model.Finding{ID: id}, nil
			},
			updateReproduceStatusFn: func(ctx context.Context, id, status string, runs int) error {
				return context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`{"runs":3}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestCreateCampaign_CreateError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, _ bool, _ int) (*model.RecordingSession, error) {
				return &model.RecordingSession{ID: id}, nil
			},
		}
		cfg.Campaigns = &mockCampaignStore{
			createFn: func(ctx context.Context, c model.Campaign) error {
				return context.DeadlineExceeded
			},
		}
	})
	body := validCampaignBody()
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestCreateCampaign_RecordingCheckError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, _ bool, _ int) (*model.RecordingSession, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	body := `{"name":"test","recording_ids":["r1"],"config":{"target":{"base_url":"http://localhost"}}}`
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestDeleteRecording_DeleteError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			deleteFn: func(ctx context.Context, id string) (bool, error) {
				return false, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/22222222-2222-2222-2222-222222222222", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetCampaignStats_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetCampaignConfig_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/config", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestSpaHandler_CSSAsset(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<html></html>")},
		"assets/style.css": &fstest.MapFile{Data: []byte("body{}")},
	}
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.WebFS = fs.FS(webFS)
	})
	req := httptest.NewRequest("GET", "/ui/assets/style.css", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/css") {
		t.Errorf("Content-Type = %q, expected text/css", ct)
	}
}

func TestParsePagination_Defaults(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listFn: func(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				if limit != 50 {
					t.Errorf("default limit = %d, want 50", limit)
				}
				if offset != 0 {
					t.Errorf("default offset = %d, want 0", offset)
				}
				return nil, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
}

func TestParsePagination_Custom(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listFn: func(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				if limit != 10 {
					t.Errorf("limit = %d, want 10", limit)
				}
				if offset != 20 {
					t.Errorf("offset = %d, want 20", offset)
				}
				return nil, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings?limit=10&offset=20", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
}

func TestParsePagination_LimitCap(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listFn: func(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				if limit != 50 {
					t.Errorf("limit = %d, want 50 (capped)", limit)
				}
				return nil, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings?limit=5000", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
}

func TestMetricsEndpoint(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetFindingArtifact_Success(t *testing.T) {
	dir := t.TempDir()
	artifactData := []byte(`{"finding_id":"f1"}`)
	if err := os.WriteFile(dir+"/artifact.json", artifactData, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.ArtifactDir = dir
		cfg.Artifacts = &mockArtifactStore{
			getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
				return &model.Artifact{ID: "a1", FilePath: "artifact.json"}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333/artifact", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetRecording_WithIncludeEntries(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error) {
				if !includeEntries {
					t.Error("expected includeEntries=true")
				}
				if maxBodyBytes != 1024 {
					t.Errorf("maxBodyBytes = %d, want 1024", maxBodyBytes)
				}
				return &model.RecordingSession{ID: id}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/22222222-2222-2222-2222-222222222222?include_entries=true&max_body_bytes=1024", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetCampaignFindings_WithFilters(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = campaignExistsStore()
		cfg.Findings = &mockFindingStore{
			listAllFn: func(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error) {
				if typeFilter != "TIMEOUT" {
					t.Errorf("typeFilter = %q, want TIMEOUT", typeFilter)
				}
				if statusFilter != "CONFIRMED" {
					t.Errorf("statusFilter = %q, want CONFIRMED", statusFilter)
				}
				return []model.Finding{{ID: "f1"}}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/findings?type=TIMEOUT&status=CONFIRMED", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestGetCampaignFindings_WithInvalidSince(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = campaignExistsStore()
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/findings?since=invalid-date", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_SINCE") {
		t.Errorf("response body does not contain INVALID_SINCE error code: %s", body)
	}
}

func TestReproduceFinding_AlreadyRunning(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return &model.Finding{ID: id, ReproduceStatus: "RUNNING"}, nil
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`{"runs":3}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestReproduceFinding_GetError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Findings = &mockFindingStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Finding, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/findings/33333333-3333-3333-3333-333333333333/reproduce", strings.NewReader(`{"runs":3}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestStartCampaign_GetError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/start", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestStopCampaign_GetError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stop", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestStreamCampaignStats_GetError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stream", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetCampaignStats_FindingsError(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{ID: id, Status: model.CampaignRunning}, nil
			},
		}
		cfg.Findings = &mockFindingStore{
			countByTypeFn: func(ctx context.Context, campaignID string) (map[model.FindingType]int, error) {
				return nil, context.DeadlineExceeded
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetCampaignStats_AllFindingTypes(t *testing.T) {
	now := time.Now()
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{
					ID:           id,
					Status:       model.CampaignRunning,
					StartedAt:    &now,
					TestsDone:    10,
					RecordingIDs: []string{},
				}, nil
			},
		}
		cfg.Findings = &mockFindingStore{
			countByTypeFn: func(ctx context.Context, campaignID string) (map[model.FindingType]int, error) {
				return map[model.FindingType]int{
					model.FindingTimeout:           1,
					model.FindingServerError:       1,
					model.FindingLatencyRegression: 1,
					model.FindingRegexMatch:        1,
				}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/stats", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestClassifyDBError_Timeout(t *testing.T) {
	got := classifyDBError(context.DeadlineExceeded)
	if got != "timeout" {
		t.Errorf("classifyDBError(DeadlineExceeded) = %q, want timeout", got)
	}
}

func TestClassifyDBError_Canceled(t *testing.T) {
	got := classifyDBError(context.Canceled)
	if got != "canceled" {
		t.Errorf("classifyDBError(Canceled) = %q, want canceled", got)
	}
}

func TestClassifyDBError_ConnectionRefused(t *testing.T) {
	err := errors.New("dial tcp 127.0.0.1:5432: connection refused")
	got := classifyDBError(err)
	if got != "connection_refused" {
		t.Errorf("classifyDBError(connection refused) = %q, want connection_refused", got)
	}
}

func TestClassifyDBError_ConnectionReset(t *testing.T) {
	err := errors.New("read tcp: connection reset by peer")
	got := classifyDBError(err)
	if got != "connection_reset" {
		t.Errorf("classifyDBError(connection reset) = %q, want connection_reset", got)
	}
}

func TestClassifyDBError_Unknown(t *testing.T) {
	err := errors.New("some unknown database error")
	got := classifyDBError(err)
	if got != "unavailable" {
		t.Errorf("classifyDBError(unknown) = %q, want unavailable", got)
	}
}

func TestExportRecordings_Empty(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listAllFn: func(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				return []model.RecordingSession{}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/export", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "recordings-export.json") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestExportRecordings_WithFilter(t *testing.T) {
	var capturedHost, capturedPath string
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listAllFn: func(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				capturedHost = hostFilter
				capturedPath = pathPrefix
				return []model.RecordingSession{{ID: "s1"}}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/export?host=example.com&path_prefix=/api", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if capturedHost != "example.com" {
		t.Errorf("host = %q", capturedHost)
	}
	if capturedPath != "/api" {
		t.Errorf("path = %q", capturedPath)
	}
}

func TestExportRecordings_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			listAllFn: func(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
				return nil, errors.New("db error")
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/export", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetRecordingsTree_OK(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getTreeFn: func(ctx context.Context) ([]model.TreeEntry, error) {
				return []model.TreeEntry{
					{Scheme: "https", Host: "api.example.com", Port: 443, Path: "/api/users", Count: 5},
					{Scheme: "https", Host: "api.example.com", Port: 443, Path: "/api/orders", Count: 3},
				}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/tree", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var tree []treeOrigin
	if err := json.Unmarshal(w.Body.Bytes(), &tree); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(tree) != 1 {
		t.Fatalf("expected 1 origin, got %d", len(tree))
	}
	if tree[0].RecordingCount != 8 {
		t.Errorf("recording count = %d, want 8", tree[0].RecordingCount)
	}
}

func TestGetRecordingsTree_Error(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getTreeFn: func(ctx context.Context) ([]model.TreeEntry, error) {
				return nil, errors.New("db error")
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/tree", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func TestGetRecordingsTree_Empty(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getTreeFn: func(ctx context.Context) ([]model.TreeEntry, error) {
				return nil, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/recordings/tree", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestBuildTree_MultipleOrigins(t *testing.T) {
	entries := []model.TreeEntry{
		{Scheme: "http", Host: "a.com", Port: 80, Path: "/x", Count: 1},
		{Scheme: "https", Host: "b.com", Port: 443, Path: "/y", Count: 2},
	}
	tree := buildTree(entries)
	if len(tree) != 2 {
		t.Fatalf("expected 2 origins, got %d", len(tree))
	}
}

func TestBuildTree_NestedPaths(t *testing.T) {
	entries := []model.TreeEntry{
		{Scheme: "https", Host: "api.com", Port: 443, Path: "/api/v1/users", Count: 3},
		{Scheme: "https", Host: "api.com", Port: 443, Path: "/api/v1/orders", Count: 2},
		{Scheme: "https", Host: "api.com", Port: 443, Path: "/health", Count: 1},
	}
	tree := buildTree(entries)
	if len(tree) != 1 {
		t.Fatalf("expected 1 origin, got %d", len(tree))
	}
	if tree[0].RecordingCount != 6 {
		t.Errorf("total count = %d, want 6", tree[0].RecordingCount)
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		path string
		want int
	}{
		{"/api/v1/users", 3},
		{"/", 0},
		{"", 0},
		{"/a", 1},
		{"///multiple///slashes///", 2},
	}
	for _, tt := range tests {
		segs := splitPath(tt.path)
		if len(segs) != tt.want {
			t.Errorf("splitPath(%q) = %d segments, want %d", tt.path, len(segs), tt.want)
		}
	}
}

func TestDeleteRecordingsByPrefix_MissingParams(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/by-prefix", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteRecordingsByPrefix_InvalidPort(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/by-prefix?scheme=http&host=a.com&port=abc", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteRecordingsByPrefix_OK(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			deleteByPrefixFn: func(ctx context.Context, scheme, host string, port int, pathPrefix string) (int64, error) {
				return 3, nil
			},
		}
	})
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/by-prefix?scheme=http&host=a.com&port=80&path_prefix=/api", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAddRecordingsToCampaign_NotFound(t *testing.T) {
	srv := newTestServer()
	body := strings.NewReader(`{"scheme":"http","host":"a.com","port":80}`)
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/recordings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestAddRecordingsToCampaign_ActiveCampaign(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{ID: id, Status: model.CampaignRunning}, nil
			},
		}
	})
	body := strings.NewReader(`{"scheme":"http","host":"a.com","port":80}`)
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/recordings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 409 {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestAddRecordingsToCampaign_OK(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = &mockCampaignStore{
			getByIDFn: func(ctx context.Context, id string) (*model.Campaign, error) {
				return &model.Campaign{ID: id, Status: model.CampaignCreated}, nil
			},
			addRecordingsByFilterFn: func(ctx context.Context, campaignID, scheme, host string, port int, pathPrefix string) (int, error) {
				return 5, nil
			},
		}
	})
	body := strings.NewReader(`{"scheme":"http","host":"a.com","port":80}`)
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/recordings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestAddRecordingsToCampaign_InvalidBody(t *testing.T) {
	srv := newTestServer()
	body := strings.NewReader(`not json`)
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/recordings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- Negative validation tests ---

func TestGetCampaign_InvalidUUID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/campaigns/not-a-uuid", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_ID") {
		t.Errorf("expected INVALID_ID in body: %s", body)
	}
}

func TestGetRecording_InvalidUUID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/recordings/not-a-uuid", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetFinding_InvalidUUID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings/not-a-uuid", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestGetFindingArtifact_PathTraversal(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.ArtifactDir = "/tmp/safe-dir"
		cfg.Artifacts = &mockArtifactStore{
			getByFindingIDFn: func(ctx context.Context, findingID string) (*model.Artifact, error) {
				return &model.Artifact{ID: "a1", FilePath: "../../etc/passwd"}, nil
			},
		}
	})
	req := httptest.NewRequest("GET", "/api/v1/findings/33333333-3333-3333-3333-333333333333/artifact", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_PATH") {
		t.Errorf("expected INVALID_PATH in body: %s", body)
	}
}

func TestListCampaigns_InvalidStatus(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/campaigns?status=BOGUS", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_STATUS") {
		t.Errorf("expected INVALID_STATUS in body: %s", body)
	}
}

func TestListFindings_InvalidType(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings?type=BOGUS", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_TYPE") {
		t.Errorf("expected INVALID_TYPE in body: %s", body)
	}
}

func TestListFindings_InvalidCampaignID(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("GET", "/api/v1/findings?campaign_id=not-uuid", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_CAMPAIGN_ID") {
		t.Errorf("expected INVALID_CAMPAIGN_ID in body: %s", body)
	}
}

func TestDeleteRecordingsByPrefix_InvalidScheme(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/by-prefix?scheme=ftp&host=a.com&port=80", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_SCHEME") {
		t.Errorf("expected INVALID_SCHEME in body: %s", body)
	}
}

func TestDeleteRecordingsByPrefix_InvalidPortRange(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest("DELETE", "/api/v1/recordings/by-prefix?scheme=http&host=a.com&port=99999", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "INVALID_PORT") {
		t.Errorf("expected INVALID_PORT in body: %s", body)
	}
}

func TestCreateCampaign_NameTooLong(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, _ bool, _ int) (*model.RecordingSession, error) {
				return &model.RecordingSession{ID: id}, nil
			},
		}
	})
	longName := strings.Repeat("a", 300)
	body := `{"name":"` + longName + `","recording_ids":["r1"],"config":{"target":{"base_url":"http://localhost"},"limits":{"workers":2,"rps":10,"req_timeout_ms":5000,"max_tests":100},"mutations":{"intensity":0.5}}}`
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "NAME_TOO_LONG") {
		t.Errorf("expected NAME_TOO_LONG in body: %s", respBody)
	}
}

func TestAddRecordingsToCampaign_InvalidScheme(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = campaignExistsStore()
	})
	body := strings.NewReader(`{"scheme":"ftp","host":"a.com","port":80}`)
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/recordings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAddRecordingsToCampaign_InvalidPort(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Campaigns = campaignExistsStore()
	})
	body := strings.NewReader(`{"scheme":"http","host":"a.com","port":99999}`)
	req := httptest.NewRequest("POST", "/api/v1/campaigns/11111111-1111-1111-1111-111111111111/recordings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestCreateCampaign_InvalidBaseURLScheme(t *testing.T) {
	srv := newTestServer(func(cfg *ServerConfig) {
		cfg.Recordings = &mockRecordingStore{
			getByIDFn: func(ctx context.Context, id string, _ bool, _ int) (*model.RecordingSession, error) {
				return &model.RecordingSession{ID: id}, nil
			},
		}
	})
	body := `{"name":"test","recording_ids":["r1"],"config":{"target":{"base_url":"ftp://evil.com"},"limits":{"workers":2,"rps":10,"req_timeout_ms":5000,"max_tests":100},"mutations":{"intensity":0.5}}}`
	req := httptest.NewRequest("POST", "/api/v1/campaigns", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != 422 {
		t.Errorf("status = %d, want 422", w.Code)
	}
}
