package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

func newTestDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlx.NewDb(db, "sqlmock"), mock
}

func TestArtifactStore_Create_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewArtifactStore(db, zerolog.Nop())

	mock.ExpectExec("INSERT INTO artifacts").
		WithArgs("art-1", "find-1", "/tmp/art.json", sqlmock.AnyArg(), int64(1024)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.Create(context.Background(), model.Artifact{
		ID: "art-1", FindingID: "find-1", FilePath: "/tmp/art.json",
		CreatedAt: time.Now(), SizeBytes: 1024,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestArtifactStore_Create_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewArtifactStore(db, zerolog.Nop())

	mock.ExpectExec("INSERT INTO artifacts").
		WillReturnError(errors.New("db error"))

	err := s.Create(context.Background(), model.Artifact{ID: "art-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestArtifactStore_GetByFindingID_Found(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewArtifactStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "finding_id", "file_path", "created_at", "size_bytes"}).
		AddRow("art-1", "find-1", "/tmp/art.json", now, int64(512))
	mock.ExpectQuery("SELECT .+ FROM artifacts WHERE finding_id").
		WithArgs("find-1").
		WillReturnRows(rows)

	a, err := s.GetByFindingID(context.Background(), "find-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil {
		t.Fatal("expected artifact, got nil")
	}
	if a.ID != "art-1" {
		t.Errorf("ID = %q, want art-1", a.ID)
	}
}

func TestArtifactStore_GetByFindingID_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewArtifactStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM artifacts WHERE finding_id").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	a, err := s.GetByFindingID(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != nil {
		t.Errorf("expected nil artifact, got %+v", a)
	}
}

func TestArtifactStore_GetByFindingID_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewArtifactStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM artifacts WHERE finding_id").
		WithArgs("find-1").
		WillReturnError(errors.New("db error"))

	_, err := s.GetByFindingID(context.Background(), "find-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_Create_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	now := time.Now().UTC()
	c := model.Campaign{
		ID: "camp-1", Name: "test", Status: model.CampaignCreated,
		CreatedAt: now, UpdatedAt: now,
		RecordingIDs: []string{"rec-1", "rec-2"},
	}

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO campaigns").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_recordings").
		WithArgs("camp-1", "rec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_recordings").
		WithArgs("camp-1", "rec-2").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := s.Create(context.Background(), c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCampaignStore_Create_InsertError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO campaigns").
		WillReturnError(errors.New("insert error"))
	mock.ExpectRollback()

	err := s.Create(context.Background(), model.Campaign{ID: "camp-1", Status: model.CampaignCreated})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_Create_LinkRecordingError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO campaigns").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO campaign_recordings").
		WillReturnError(errors.New("link error"))
	mock.ExpectRollback()

	err := s.Create(context.Background(), model.Campaign{
		ID: "camp-1", Status: model.CampaignCreated, RecordingIDs: []string{"rec-1"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_Create_BeginError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectBegin().WillReturnError(errors.New("begin error"))

	err := s.Create(context.Background(), model.Campaign{ID: "camp-1", Status: model.CampaignCreated})
	if err == nil {
		t.Fatal("expected error")
	}
}

func campaignColumns() []string {
	return []string{
		"id", "name", "status", "created_at", "updated_at",
		"started_at", "finished_at", "config", "tests_done", "findings_total",
	}
}

func TestCampaignStore_GetByID_Found(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	now := time.Now().UTC()
	cfg := `{"target":{"base_url":"http://example.com"}}`

	rows := sqlmock.NewRows(campaignColumns()).
		AddRow("camp-1", "test", "RUNNING", now, now, nil, nil, []byte(cfg), 42, 3)
	mock.ExpectQuery("SELECT .+ FROM campaigns WHERE id").
		WithArgs("camp-1").
		WillReturnRows(rows)

	recRows := sqlmock.NewRows([]string{"recording_id"}).AddRow("rec-1").AddRow("rec-2")
	mock.ExpectQuery("SELECT recording_id FROM campaign_recordings").
		WithArgs("camp-1").
		WillReturnRows(recRows)

	c, err := s.GetByID(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected campaign, got nil")
	}
	if c.ID != "camp-1" {
		t.Errorf("ID = %q, want camp-1", c.ID)
	}
	if len(c.RecordingIDs) != 2 {
		t.Errorf("RecordingIDs len = %d, want 2", len(c.RecordingIDs))
	}
}

func TestCampaignStore_GetByID_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM campaigns WHERE id").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	c, err := s.GetByID(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Errorf("expected nil campaign, got %+v", c)
	}
}

func TestCampaignStore_GetByID_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM campaigns WHERE id").
		WithArgs("camp-1").
		WillReturnError(errors.New("db error"))

	_, err := s.GetByID(context.Background(), "camp-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_GetByID_RecordingIDsError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(campaignColumns()).
		AddRow("camp-1", "test", "RUNNING", now, now, nil, nil, []byte(`{}`), 0, 0)
	mock.ExpectQuery("SELECT .+ FROM campaigns WHERE id").
		WithArgs("camp-1").
		WillReturnRows(rows)
	mock.ExpectQuery("SELECT recording_id FROM campaign_recordings").
		WithArgs("camp-1").
		WillReturnError(errors.New("db error"))

	_, err := s.GetByID(context.Background(), "camp-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_List_NoFilter(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(campaignColumns()).
		AddRow("camp-1", "first", "RUNNING", now, now, nil, nil, []byte(`{}`), 10, 1).
		AddRow("camp-2", "second", "FINISHED", now, now, nil, nil, []byte(`{}`), 20, 2)
	mock.ExpectQuery("SELECT .+ FROM campaigns ORDER BY").
		WithArgs(50, 0).
		WillReturnRows(rows)

	list, err := s.List(context.Background(), "", 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 campaigns, got %d", len(list))
	}
}

func TestCampaignStore_List_WithFilter(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(campaignColumns()).
		AddRow("camp-1", "first", "RUNNING", now, now, nil, nil, []byte(`{}`), 10, 1)
	mock.ExpectQuery("SELECT .+ FROM campaigns WHERE status").
		WithArgs("RUNNING", 50, 0).
		WillReturnRows(rows)

	list, err := s.List(context.Background(), "RUNNING", 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 campaign, got %d", len(list))
	}
}

func TestCampaignStore_List_DefaultLimit(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	rows := sqlmock.NewRows(campaignColumns())
	mock.ExpectQuery("SELECT .+ FROM campaigns ORDER BY").
		WithArgs(50, 0).
		WillReturnRows(rows)

	_, err := s.List(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCampaignStore_List_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM campaigns").
		WillReturnError(errors.New("db error"))

	_, err := s.List(context.Background(), "", 50, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_UpdateStatus_Running(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignCreated, model.CampaignRunning)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
}

func TestCampaignStore_UpdateStatus_Finished(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignRunning, model.CampaignFinished)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
}

func TestCampaignStore_UpdateStatus_Default(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignRunning, model.CampaignStopping)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
}

func TestCampaignStore_UpdateStatus_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ok, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignCreated, model.CampaignRunning)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false when no rows affected")
	}
}

func TestCampaignStore_UpdateStatus_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnError(errors.New("db error"))

	_, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignCreated, model.CampaignRunning)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_IncrementStats_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET tests_done").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.IncrementStats(context.Background(), "camp-1", 5, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCampaignStore_IncrementStats_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET tests_done").
		WillReturnError(errors.New("db error"))

	err := s.IncrementStats(context.Background(), "camp-1", 5, 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func findingColumns() []string {
	return []string{
		"id", "campaign_id", "type", "status", "signature", "created_at", "confirmed_at",
		"method", "endpoint", "details", "seed_recording_id", "minimized",
		"reproduce_status", "reproduce_enqueued_at",
		"severity", "owasp_category", "group_id", "reproducibility",
	}
}

func TestFindingStore_Create_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("INSERT INTO findings").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.Create(context.Background(), model.Finding{
		ID: "find-1", CampaignID: "camp-1", Type: model.FindingTimeout,
		Status: model.FindingUnconfirmed, Signature: "sig-1", CreatedAt: time.Now(),
		Method: "GET", Endpoint: "/api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_Create_WithSeedRecording(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("INSERT INTO findings").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.Create(context.Background(), model.Finding{
		ID: "find-1", CampaignID: "camp-1", Type: model.FindingServerError,
		Status: model.FindingUnconfirmed, SeedRecordingID: "rec-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_Create_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("INSERT INTO findings").
		WillReturnError(errors.New("db error"))

	err := s.Create(context.Background(), model.Finding{ID: "find-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_GetByID_Found(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	details, _ := json.Marshal(model.FindingDetails{HTTPStatus: 500})

	rows := sqlmock.NewRows(findingColumns()).
		AddRow("find-1", "camp-1", "SERVER_ERROR", "CONFIRMED", "sig-1", now, nil,
			"GET", "/api/crash", details, nil, true, nil, nil,
			"MEDIUM", "A06_INSECURE_DESIGN", nil, 0.0)
	mock.ExpectQuery("SELECT .+ FROM findings WHERE id").
		WithArgs("find-1").
		WillReturnRows(rows)

	// loadArtifactID sub-query
	artRows := sqlmock.NewRows([]string{"id"}).AddRow("art-1")
	mock.ExpectQuery("SELECT id FROM artifacts WHERE finding_id").
		WithArgs("find-1").
		WillReturnRows(artRows)

	f, err := s.GetByID(context.Background(), "find-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("expected finding, got nil")
	}
	if f.ID != "find-1" {
		t.Errorf("ID = %q, want find-1", f.ID)
	}
	if f.ArtifactID != "art-1" {
		t.Errorf("ArtifactID = %q, want art-1", f.ArtifactID)
	}
}

func TestFindingStore_GetByID_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM findings WHERE id").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	f, err := s.GetByID(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f != nil {
		t.Errorf("expected nil finding, got %+v", f)
	}
}

func TestFindingStore_GetByID_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM findings WHERE id").
		WithArgs("find-1").
		WillReturnError(errors.New("db error"))

	_, err := s.GetByID(context.Background(), "find-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_ExistsBySignature_Exists(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("camp-1", "sig-1").
		WillReturnRows(rows)

	exists, err := s.ExistsBySignature(context.Background(), "camp-1", "sig-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected exists=true")
	}
}

func TestFindingStore_ExistsBySignature_NotExists(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("camp-1", "sig-new").
		WillReturnRows(rows)

	exists, err := s.ExistsBySignature(context.Background(), "camp-1", "sig-new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected exists=false")
	}
}

func TestFindingStore_ExistsBySignature_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT EXISTS").
		WillReturnError(errors.New("db error"))

	_, err := s.ExistsBySignature(context.Background(), "camp-1", "sig-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_UpdateReproduceStatus_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE findings SET reproduce_status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.UpdateReproduceStatus(context.Background(), "find-1", "RUNNING", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_UpdateReproduceStatus_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE findings SET reproduce_status").
		WillReturnError(errors.New("db error"))

	err := s.UpdateReproduceStatus(context.Background(), "find-1", "RUNNING", 3)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_UpdateStatus_Confirmed(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE findings SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.UpdateStatus(context.Background(), "find-1", model.FindingConfirmed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_UpdateStatus_Unconfirmed(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE findings SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.UpdateStatus(context.Background(), "find-1", model.FindingUnconfirmed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_UpdateStatus_ConfirmedError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE findings SET status").
		WillReturnError(errors.New("db error"))

	err := s.UpdateStatus(context.Background(), "find-1", model.FindingConfirmed)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_UpdateStatus_UnconfirmedError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE findings SET status").
		WillReturnError(errors.New("db error"))

	err := s.UpdateStatus(context.Background(), "find-1", model.FindingUnconfirmed)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_UpdateFindingGroup_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	findingID := "find-1"
	groupID := "group-1"

	mock.ExpectExec("UPDATE findings SET group_id").
		WithArgs(groupID, findingID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := s.UpdateFindingGroup(context.Background(), findingID, groupID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_UpdateFindingGroup_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE findings SET group_id").
		WillReturnError(errors.New("db error"))

	err := s.UpdateFindingGroup(context.Background(), "find-1", "group-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_ListAll_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	details, _ := json.Marshal(model.FindingDetails{})

	rows := sqlmock.NewRows(findingColumns()).
		AddRow("find-1", "camp-1", "TIMEOUT", "UNCONFIRMED", "sig-1", now, nil,
			"GET", "/api", details, nil, false, nil, nil,
			"INFO", "UNCATEGORIZED", nil, 0.0)
	mock.ExpectQuery("SELECT .+ FROM findings f").
		WillReturnRows(rows)

	// batchLoadArtifactIDs sub-query
	artRows := sqlmock.NewRows([]string{"finding_id", "id"}).AddRow("find-1", "art-1")
	mock.ExpectQuery("SELECT DISTINCT ON .+ FROM artifacts").
		WillReturnRows(artRows)

	findings, err := s.ListAll(context.Background(), "", "", "", nil, 50, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].ArtifactID != "art-1" {
		t.Errorf("ArtifactID = %q, want art-1", findings[0].ArtifactID)
	}
}

func TestFindingStore_ListAll_DefaultLimit(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows(findingColumns())
	mock.ExpectQuery("SELECT .+ FROM findings f").
		WillReturnRows(rows)

	_, err := s.ListAll(context.Background(), "", "", "", nil, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_ListAll_WithFilters(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	since := time.Now().Add(-time.Hour)
	rows := sqlmock.NewRows(findingColumns())
	mock.ExpectQuery("SELECT .+ FROM findings f WHERE").
		WillReturnRows(rows)

	_, err := s.ListAll(context.Background(), "camp-1", "TIMEOUT", "CONFIRMED", &since, 25, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFindingStore_ListAll_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM findings f").
		WillReturnError(errors.New("db error"))

	_, err := s.ListAll(context.Background(), "", "", "", nil, 50, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_CountByType_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows([]string{"type", "cnt"}).
		AddRow("TIMEOUT", 5).
		AddRow("SERVER_ERROR", 3)
	mock.ExpectQuery("SELECT type, COUNT").
		WithArgs("camp-1").
		WillReturnRows(rows)

	counts, err := s.CountByType(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if counts[model.FindingTimeout] != 5 {
		t.Errorf("TIMEOUT count = %d, want 5", counts[model.FindingTimeout])
	}
	if counts[model.FindingServerError] != 3 {
		t.Errorf("SERVER_ERROR count = %d, want 3", counts[model.FindingServerError])
	}
}

func TestFindingStore_CountByType_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT type, COUNT").
		WillReturnError(errors.New("db error"))

	_, err := s.CountByType(context.Background(), "camp-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFindingStore_LoadArtifactID_Found(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows([]string{"id"}).AddRow("art-1")
	mock.ExpectQuery("SELECT id FROM artifacts WHERE finding_id").
		WithArgs("find-1").
		WillReturnRows(rows)

	id := s.loadArtifactID(context.Background(), "find-1")
	if id != "art-1" {
		t.Errorf("artifact ID = %q, want art-1", id)
	}
}

func TestFindingStore_LoadArtifactID_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT id FROM artifacts WHERE finding_id").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	id := s.loadArtifactID(context.Background(), "missing")
	if id != "" {
		t.Errorf("expected empty string, got %q", id)
	}
}

func TestFindingStore_BatchLoadArtifactIDs_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows([]string{"finding_id", "id"}).
		AddRow("find-1", "art-1").
		AddRow("find-2", "art-2")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT DISTINCT ON (finding_id) finding_id, id FROM artifacts")).
		WillReturnRows(rows)

	m, err := s.batchLoadArtifactIDs(context.Background(), []string{"find-1", "find-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["find-1"] != "art-1" {
		t.Errorf("find-1 artifact = %q, want art-1", m["find-1"])
	}
	if m["find-2"] != "art-2" {
		t.Errorf("find-2 artifact = %q, want art-2", m["find-2"])
	}
}

func TestFindingStore_BatchLoadArtifactIDs_Empty(t *testing.T) {
	db, _ := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	m, err := s.batchLoadArtifactIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil map, got %v", m)
	}
}

func TestFindingStore_BatchLoadArtifactIDs_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewFindingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT DISTINCT").
		WillReturnError(errors.New("db error"))

	_, err := s.batchLoadArtifactIDs(context.Background(), []string{"find-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func recordingColumns() []string {
	return []string{
		"id", "schema_version", "created_at", "target_scheme", "target_host", "target_port", "target_path", "entry_count",
	}
}

func TestRecordingStore_Upsert_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	sess := model.RecordingSession{
		ID: "rec-1", SchemaVersion: 1, CreatedAt: now,
		Target: model.TargetInfo{Scheme: "https", Host: "example.com", Port: 443},
		Entries: []model.Exchange{
			{
				RequestID: "req-1", StartedAt: now, DurationMs: 100,
				Request:  model.RequestData{Method: "GET", Path: "/api", Headers: map[string][]string{"X-Test": {"1"}}},
				Response: model.ResponseData{Status: 200, Headers: map[string][]string{"Content-Type": {"application/json"}}},
			},
		},
	}

	mock.ExpectBegin()
	existsRows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("rec-1").
		WillReturnRows(existsRows)
	mock.ExpectExec("INSERT INTO recordings").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO exchanges").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	inserted, err := s.Upsert(context.Background(), sess)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !inserted {
		t.Error("expected inserted=true")
	}
}

func TestRecordingStore_Upsert_AlreadyExists(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectBegin()
	existsRows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("rec-1").
		WillReturnRows(existsRows)
	mock.ExpectRollback()

	inserted, err := s.Upsert(context.Background(), model.RecordingSession{ID: "rec-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inserted {
		t.Error("expected inserted=false for existing recording")
	}
}

func TestRecordingStore_Upsert_BeginError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectBegin().WillReturnError(errors.New("begin error"))

	_, err := s.Upsert(context.Background(), model.RecordingSession{ID: "rec-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordingStore_Upsert_CheckExistsError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS").
		WillReturnError(errors.New("query error"))
	mock.ExpectRollback()

	_, err := s.Upsert(context.Background(), model.RecordingSession{ID: "rec-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordingStore_Upsert_InsertRecordingError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectBegin()
	existsRows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(existsRows)
	mock.ExpectExec("INSERT INTO recordings").
		WillReturnError(errors.New("insert error"))
	mock.ExpectRollback()

	_, err := s.Upsert(context.Background(), model.RecordingSession{ID: "rec-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordingStore_Upsert_InsertExchangeError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	sess := model.RecordingSession{
		ID:      "rec-1",
		Entries: []model.Exchange{{RequestID: "req-1", Request: model.RequestData{Headers: map[string][]string{}}, Response: model.ResponseData{Headers: map[string][]string{}}}},
	}

	mock.ExpectBegin()
	existsRows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery("SELECT EXISTS").WillReturnRows(existsRows)
	mock.ExpectExec("INSERT INTO recordings").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO exchanges").
		WillReturnError(errors.New("exchange insert error"))
	mock.ExpectRollback()

	_, err := s.Upsert(context.Background(), sess)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordingStore_List_NoFilter(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(recordingColumns()).
		AddRow("rec-1", 1, now, "https", "example.com", 443, "/api", 5).
		AddRow("rec-2", 1, now, "http", "localhost", 8080, "/", 3)
	mock.ExpectQuery("SELECT .+ FROM recordings ORDER BY").
		WithArgs(50, 0).
		WillReturnRows(rows)

	list, err := s.List(context.Background(), 50, 0, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 recordings, got %d", len(list))
	}
}

func TestRecordingStore_List_WithHostFilter(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(recordingColumns()).
		AddRow("rec-1", 1, now, "https", "example.com", 443, "/api", 5)
	mock.ExpectQuery("SELECT .+ FROM recordings WHERE target_host").
		WithArgs("example.com", 50, 0).
		WillReturnRows(rows)

	list, err := s.List(context.Background(), 50, 0, "example.com", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 recording, got %d", len(list))
	}
}

func TestRecordingStore_List_DefaultLimit(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows(recordingColumns())
	mock.ExpectQuery("SELECT .+ FROM recordings ORDER BY").
		WithArgs(50, 0).
		WillReturnRows(rows)

	_, err := s.List(context.Background(), 0, 0, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecordingStore_List_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM recordings").
		WillReturnError(errors.New("db error"))

	_, err := s.List(context.Background(), 50, 0, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func exchangeColumns() []string {
	return []string{
		"id", "recording_id", "request_id", "started_at", "duration_ms",
		"req_method", "req_path", "req_query", "req_headers", "req_body_b64", "req_body_truncated",
		"resp_status", "resp_headers", "resp_body_b64", "resp_body_truncated", "seq_order",
	}
}

func TestRecordingStore_GetByID_Found_WithEntries(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(recordingColumns()).
		AddRow("rec-1", 1, now, "https", "example.com", 443, "/api", 1)
	mock.ExpectQuery("SELECT .+ FROM recordings WHERE id").
		WithArgs("rec-1").
		WillReturnRows(rows)

	reqH, _ := json.Marshal(map[string][]string{"Content-Type": {"text/plain"}})
	respH, _ := json.Marshal(map[string][]string{})
	exRows := sqlmock.NewRows(exchangeColumns()).
		AddRow("ex-1", "rec-1", "req-1", now, 150, "GET", "/api", "q=1", reqH, "body", false, 200, respH, "resp", false, 0)
	mock.ExpectQuery("SELECT .+ FROM exchanges WHERE recording_id").
		WithArgs("rec-1").
		WillReturnRows(exRows)

	sess, err := s.GetByID(context.Background(), "rec-1", true, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if len(sess.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(sess.Entries))
	}
	if sess.Entries[0].Request.Method != "GET" {
		t.Errorf("Method = %q, want GET", sess.Entries[0].Request.Method)
	}
}

func TestRecordingStore_GetByID_Found_WithoutEntries(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(recordingColumns()).
		AddRow("rec-1", 1, now, "https", "example.com", 443, "/", 1)
	mock.ExpectQuery("SELECT .+ FROM recordings WHERE id").
		WithArgs("rec-1").
		WillReturnRows(rows)

	sess, err := s.GetByID(context.Background(), "rec-1", false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess == nil {
		t.Fatal("expected session, got nil")
	}
	if sess.Entries != nil {
		t.Errorf("expected nil entries, got %d", len(sess.Entries))
	}
}

func TestRecordingStore_GetByID_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM recordings WHERE id").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	sess, err := s.GetByID(context.Background(), "missing", false, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sess != nil {
		t.Errorf("expected nil, got %+v", sess)
	}
}

func TestRecordingStore_GetByID_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT .+ FROM recordings WHERE id").
		WillReturnError(errors.New("db error"))

	_, err := s.GetByID(context.Background(), "rec-1", false, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordingStore_GetByID_ExchangeError(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	now := time.Now().UTC()
	rows := sqlmock.NewRows(recordingColumns()).
		AddRow("rec-1", 1, now, "https", "example.com", 443, "/", 1)
	mock.ExpectQuery("SELECT .+ FROM recordings WHERE id").
		WithArgs("rec-1").
		WillReturnRows(rows)
	mock.ExpectQuery("SELECT .+ FROM exchanges WHERE recording_id").
		WillReturnError(errors.New("exchange error"))

	_, err := s.GetByID(context.Background(), "rec-1", true, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordingStore_Delete_Success(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectExec("DELETE FROM recordings WHERE id").
		WithArgs("rec-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := s.Delete(context.Background(), "rec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
}

func TestRecordingStore_Delete_NotFound(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectExec("DELETE FROM recordings WHERE id").
		WithArgs("missing").
		WillReturnResult(sqlmock.NewResult(0, 0))

	ok, err := s.Delete(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false")
	}
}

func TestRecordingStore_Delete_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectExec("DELETE FROM recordings WHERE id").
		WillReturnError(errors.New("db error"))

	_, err := s.Delete(context.Background(), "rec-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRecordingStore_IsUsedByActiveCampaign_Active(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows([]string{"exists"}).AddRow(true)
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("rec-1").
		WillReturnRows(rows)

	active, err := s.IsUsedByActiveCampaign(context.Background(), "rec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Error("expected active=true")
	}
}

func TestRecordingStore_IsUsedByActiveCampaign_NotActive(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	rows := sqlmock.NewRows([]string{"exists"}).AddRow(false)
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs("rec-1").
		WillReturnRows(rows)

	active, err := s.IsUsedByActiveCampaign(context.Background(), "rec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active {
		t.Error("expected active=false")
	}
}

func TestRecordingStore_IsUsedByActiveCampaign_Error(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewRecordingStore(db, zerolog.Nop())

	mock.ExpectQuery("SELECT EXISTS").
		WillReturnError(errors.New("db error"))

	_, err := s.IsUsedByActiveCampaign(context.Background(), "rec-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCampaignStore_UpdateStatus_Starting(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignCreated, model.CampaignStarting)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
}

func TestCampaignStore_UpdateStatus_Failed(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignRunning, model.CampaignFailed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
}

func TestCampaignStore_UpdateStatus_Stopped(t *testing.T) {
	db, mock := newTestDB(t)
	s := NewCampaignStore(db, zerolog.Nop())

	mock.ExpectExec("UPDATE campaigns SET status").
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := s.UpdateStatus(context.Background(), "camp-1", model.CampaignStopping, model.CampaignStopped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok=true")
	}
}
