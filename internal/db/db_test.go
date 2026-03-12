package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestNewCampaignStore(t *testing.T) {
	s := NewCampaignStore(nil, zerolog.Nop())
	if s == nil {
		t.Fatal("expected non-nil CampaignStore")
	}
}

func TestNewRecordingStore(t *testing.T) {
	s := NewRecordingStore(nil, zerolog.Nop())
	if s == nil {
		t.Fatal("expected non-nil RecordingStore")
	}
}

func TestNewFindingStore(t *testing.T) {
	s := NewFindingStore(nil, zerolog.Nop())
	if s == nil {
		t.Fatal("expected non-nil FindingStore")
	}
}

func TestNewArtifactStore(t *testing.T) {
	s := NewArtifactStore(nil, zerolog.Nop())
	if s == nil {
		t.Fatal("expected non-nil ArtifactStore")
	}
}

func TestCampaignRow_ToModel_Basic(t *testing.T) {
	now := time.Now().UTC()
	cfg := `{"target":{"base_url":"http://example.com"}}`
	row := campaignRow{
		ID:            "camp-1",
		Name:          "test campaign",
		Status:        "RUNNING",
		CreatedAt:     now,
		UpdatedAt:     now,
		Config:        []byte(cfg),
		TestsDone:     42,
		FindingsTotal: 3,
	}

	m := row.toModel([]string{"rec-1", "rec-2"}, zerolog.Nop())
	if m.ID != "camp-1" {
		t.Errorf("ID = %q, want camp-1", m.ID)
	}
	if m.Name != "test campaign" {
		t.Errorf("Name = %q, want 'test campaign'", m.Name)
	}
	if string(m.Status) != "RUNNING" {
		t.Errorf("Status = %q, want RUNNING", m.Status)
	}
	if len(m.RecordingIDs) != 2 {
		t.Errorf("RecordingIDs len = %d, want 2", len(m.RecordingIDs))
	}
	if m.TestsDone != 42 {
		t.Errorf("TestsDone = %d, want 42", m.TestsDone)
	}
	if m.FindingsN != 3 {
		t.Errorf("FindingsN = %d, want 3", m.FindingsN)
	}
	if m.Progress == nil {
		t.Fatal("Progress should not be nil")
	}
	if m.Progress.TestsDone != 42 {
		t.Errorf("Progress.TestsDone = %d, want 42", m.Progress.TestsDone)
	}
	if m.StartedAt != nil {
		t.Error("StartedAt should be nil when NullTime is invalid")
	}
	if m.FinishedAt != nil {
		t.Error("FinishedAt should be nil when NullTime is invalid")
	}
}

func TestCampaignRow_ToModel_WithDates(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(-time.Hour)
	finished := now.Add(-time.Minute)
	row := campaignRow{
		ID:         "camp-2",
		Name:       "dated",
		Status:     "FINISHED",
		CreatedAt:  now,
		UpdatedAt:  now,
		StartedAt:  sql.NullTime{Time: started, Valid: true},
		FinishedAt: sql.NullTime{Time: finished, Valid: true},
		Config:     []byte(`{}`),
	}

	m := row.toModel(nil, zerolog.Nop())
	if m.StartedAt == nil {
		t.Fatal("StartedAt should be set")
	}
	if !m.StartedAt.Equal(started) {
		t.Errorf("StartedAt = %v, want %v", m.StartedAt, started)
	}
	if m.FinishedAt == nil {
		t.Fatal("FinishedAt should be set")
	}
	if !m.FinishedAt.Equal(finished) {
		t.Errorf("FinishedAt = %v, want %v", m.FinishedAt, finished)
	}
}

func TestCampaignRow_ToModel_InvalidConfig(t *testing.T) {
	row := campaignRow{
		ID:     "camp-bad",
		Name:   "bad config",
		Status: "CREATED",
		Config: []byte(`{not valid json`),
	}

	// Should not panic; logs a warning and continues
	m := row.toModel(nil, zerolog.Nop())
	if m.ID != "camp-bad" {
		t.Errorf("ID = %q, want camp-bad", m.ID)
	}
}

func TestRecordingRow_ToModel_Basic(t *testing.T) {
	now := time.Now().UTC()
	row := recordingRow{
		ID:            "rec-1",
		SchemaVersion: 2,
		CreatedAt:     sql.NullTime{Time: now, Valid: true},
		TargetScheme:  "https",
		TargetHost:    "api.example.com",
		TargetPort:    443,
		EntryCount:    10,
	}

	m := row.toModel()
	if m.ID != "rec-1" {
		t.Errorf("ID = %q, want rec-1", m.ID)
	}
	if m.SchemaVersion != 2 {
		t.Errorf("SchemaVersion = %d, want 2", m.SchemaVersion)
	}
	if !m.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", m.CreatedAt, now)
	}
	if m.Target.Scheme != "https" {
		t.Errorf("Target.Scheme = %q, want https", m.Target.Scheme)
	}
	if m.Target.Host != "api.example.com" {
		t.Errorf("Target.Host = %q, want api.example.com", m.Target.Host)
	}
	if m.Target.Port != 443 {
		t.Errorf("Target.Port = %d, want 443", m.Target.Port)
	}
	if m.EntryCount != 10 {
		t.Errorf("EntryCount = %d, want 10", m.EntryCount)
	}
}

func TestRecordingRow_ToModel_NullCreatedAt(t *testing.T) {
	row := recordingRow{
		ID:        "rec-null",
		CreatedAt: sql.NullTime{Valid: false},
	}
	m := row.toModel()
	if !m.CreatedAt.IsZero() {
		t.Errorf("expected zero CreatedAt for null time, got %v", m.CreatedAt)
	}
}

func TestExchangeRow_ToModel_Basic(t *testing.T) {
	now := time.Now().UTC()
	reqHeaders, _ := json.Marshal(map[string][]string{"Content-Type": {"application/json"}})
	respHeaders, _ := json.Marshal(map[string][]string{"X-Custom": {"value"}})

	row := exchangeRow{
		ID:          "ex-1",
		RecordingID: "rec-1",
		RequestID:   "req-1",
		StartedAt:   sql.NullTime{Time: now, Valid: true},
		DurationMs:  150,
		ReqMethod:   "POST",
		ReqPath:     "/api/data",
		ReqQuery:    "page=1",
		ReqHeaders:  reqHeaders,
		ReqBodyB64:  "aGVsbG8=",
		RespStatus:  201,
		RespHeaders: respHeaders,
		RespBodyB64: "b2s=",
	}

	m := row.toModel(0, zerolog.Nop())
	if m.RequestID != "req-1" {
		t.Errorf("RequestID = %q, want req-1", m.RequestID)
	}
	if m.DurationMs != 150 {
		t.Errorf("DurationMs = %d, want 150", m.DurationMs)
	}
	if m.Request.Method != "POST" {
		t.Errorf("Method = %q, want POST", m.Request.Method)
	}
	if m.Request.Path != "/api/data" {
		t.Errorf("Path = %q, want /api/data", m.Request.Path)
	}
	if m.Request.Query != "page=1" {
		t.Errorf("Query = %q, want page=1", m.Request.Query)
	}
	if m.Request.BodyB64 != "aGVsbG8=" {
		t.Errorf("ReqBodyB64 = %q, want aGVsbG8=", m.Request.BodyB64)
	}
	if m.Response.Status != 201 {
		t.Errorf("Status = %d, want 201", m.Response.Status)
	}
	if m.Response.BodyB64 != "b2s=" {
		t.Errorf("RespBodyB64 = %q, want b2s=", m.Response.BodyB64)
	}
	if !m.StartedAt.Equal(now) {
		t.Errorf("StartedAt = %v, want %v", m.StartedAt, now)
	}
	if m.Request.Headers == nil {
		t.Error("expected request headers to be parsed")
	}
	if m.Response.Headers == nil {
		t.Error("expected response headers to be parsed")
	}
}

func TestExchangeRow_ToModel_MaxBodyTruncation(t *testing.T) {
	row := exchangeRow{
		ID:          "ex-trunc",
		ReqBodyB64:  "AAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		RespBodyB64: "BBBBBBBBBBBBBBBBBBBBBBBBBBBB",
	}

	m := row.toModel(5, zerolog.Nop())
	// maxBodyBytes=5, aligned to 4-byte boundary: (5/4)*4 = 4
	if len(m.Request.BodyB64) != 4 {
		t.Errorf("req body len = %d, want 4", len(m.Request.BodyB64))
	}
	if !m.Request.BodyTruncated {
		t.Error("expected request body to be marked truncated")
	}
	if len(m.Response.BodyB64) != 4 {
		t.Errorf("resp body len = %d, want 4", len(m.Response.BodyB64))
	}
	if !m.Response.BodyTruncated {
		t.Error("expected response body to be marked truncated")
	}
}

func TestExchangeRow_ToModel_NoTruncationWhenUnderLimit(t *testing.T) {
	row := exchangeRow{
		ID:          "ex-short",
		ReqBodyB64:  "abc",
		RespBodyB64: "def",
	}

	m := row.toModel(100, zerolog.Nop())
	if m.Request.BodyB64 != "abc" {
		t.Errorf("req body = %q, want abc", m.Request.BodyB64)
	}
	if m.Request.BodyTruncated {
		t.Error("request body should not be truncated")
	}
	if m.Response.BodyB64 != "def" {
		t.Errorf("resp body = %q, want def", m.Response.BodyB64)
	}
	if m.Response.BodyTruncated {
		t.Error("response body should not be truncated")
	}
}

func TestExchangeRow_ToModel_NullStartedAt(t *testing.T) {
	row := exchangeRow{
		ID:        "ex-null",
		StartedAt: sql.NullTime{Valid: false},
	}
	m := row.toModel(0, zerolog.Nop())
	if !m.StartedAt.IsZero() {
		t.Errorf("expected zero StartedAt, got %v", m.StartedAt)
	}
}

func TestExchangeRow_ToModel_InvalidHeaders(t *testing.T) {
	row := exchangeRow{
		ID:          "ex-badhdr",
		ReqHeaders:  []byte(`{not valid}`),
		RespHeaders: []byte(`{also invalid}`),
	}
	// Should not panic; logs warnings and continues
	m := row.toModel(0, zerolog.Nop())
	if m.Request.Headers != nil {
		t.Error("expected nil request headers for invalid JSON")
	}
	if m.Response.Headers != nil {
		t.Error("expected nil response headers for invalid JSON")
	}
}

func TestExchangeRow_ToModel_EmptyHeaders(t *testing.T) {
	row := exchangeRow{
		ID:          "ex-nohdr",
		ReqHeaders:  nil,
		RespHeaders: nil,
	}
	m := row.toModel(0, zerolog.Nop())
	if m.Request.Headers != nil {
		t.Error("expected nil request headers for empty input")
	}
	if m.Response.Headers != nil {
		t.Error("expected nil response headers for empty input")
	}
}

func TestFindingRow_ToModel_Basic(t *testing.T) {
	now := time.Now().UTC()
	details, _ := json.Marshal(map[string]interface{}{
		"http_status": 500,
	})

	row := findingRow{
		ID:         "find-1",
		CampaignID: "camp-1",
		Type:       "SERVER_ERROR",
		Status:     "CONFIRMED",
		Signature:  "sig-abc",
		CreatedAt:  now,
		Method:     "GET",
		Endpoint:   "/api/crash",
		Details:    details,
		Minimized:  true,
	}

	m := row.toModel(zerolog.Nop())
	if m.ID != "find-1" {
		t.Errorf("ID = %q, want find-1", m.ID)
	}
	if m.CampaignID != "camp-1" {
		t.Errorf("CampaignID = %q, want camp-1", m.CampaignID)
	}
	if string(m.Type) != "SERVER_ERROR" {
		t.Errorf("Type = %q, want SERVER_ERROR", m.Type)
	}
	if string(m.Status) != "CONFIRMED" {
		t.Errorf("Status = %q, want CONFIRMED", m.Status)
	}
	if m.Signature != "sig-abc" {
		t.Errorf("Signature = %q, want sig-abc", m.Signature)
	}
	if m.Method != "GET" {
		t.Errorf("Method = %q, want GET", m.Method)
	}
	if m.Endpoint != "/api/crash" {
		t.Errorf("Endpoint = %q, want /api/crash", m.Endpoint)
	}
	if !m.Minimized {
		t.Error("expected Minimized=true")
	}
	if m.Details.HTTPStatus != 500 {
		t.Errorf("Details.HTTPStatus = %d, want 500", m.Details.HTTPStatus)
	}
}

func TestFindingRow_ToModel_NullFields(t *testing.T) {
	now := time.Now().UTC()
	confirmed := now.Add(-time.Minute)
	enqueued := now.Add(-30 * time.Second)

	row := findingRow{
		ID:                  "find-2",
		CampaignID:          "camp-2",
		Type:                "TIMEOUT",
		Status:              "UNCONFIRMED",
		CreatedAt:           now,
		ConfirmedAt:         sql.NullTime{Time: confirmed, Valid: true},
		SeedRecordingID:     sql.NullString{String: "rec-seed", Valid: true},
		ReproduceStatus:     sql.NullString{String: "PENDING", Valid: true},
		ReproduceEnqueuedAt: sql.NullTime{Time: enqueued, Valid: true},
		Details:             []byte(`{}`),
	}

	m := row.toModel(zerolog.Nop())
	if m.ConfirmedAt == nil || !m.ConfirmedAt.Equal(confirmed) {
		t.Errorf("ConfirmedAt = %v, want %v", m.ConfirmedAt, confirmed)
	}
	if m.SeedRecordingID != "rec-seed" {
		t.Errorf("SeedRecordingID = %q, want rec-seed", m.SeedRecordingID)
	}
	if m.ReproduceStatus != "PENDING" {
		t.Errorf("ReproduceStatus = %q, want PENDING", m.ReproduceStatus)
	}
	if m.ReproduceEnqueuedAt == nil || !m.ReproduceEnqueuedAt.Equal(enqueued) {
		t.Errorf("ReproduceEnqueuedAt = %v, want %v", m.ReproduceEnqueuedAt, enqueued)
	}
}

func TestFindingRow_ToModel_InvalidNullFields(t *testing.T) {
	row := findingRow{
		ID:                  "find-3",
		CampaignID:          "camp-3",
		Type:                "REGEX_MATCH",
		Status:              "UNCONFIRMED",
		ConfirmedAt:         sql.NullTime{Valid: false},
		SeedRecordingID:     sql.NullString{Valid: false},
		ReproduceStatus:     sql.NullString{Valid: false},
		ReproduceEnqueuedAt: sql.NullTime{Valid: false},
	}

	m := row.toModel(zerolog.Nop())
	if m.ConfirmedAt != nil {
		t.Error("expected nil ConfirmedAt")
	}
	if m.SeedRecordingID != "" {
		t.Errorf("expected empty SeedRecordingID, got %q", m.SeedRecordingID)
	}
	if m.ReproduceStatus != "" {
		t.Errorf("expected empty ReproduceStatus, got %q", m.ReproduceStatus)
	}
	if m.ReproduceEnqueuedAt != nil {
		t.Error("expected nil ReproduceEnqueuedAt")
	}
}

func TestFindingRow_ToModel_InvalidDetails(t *testing.T) {
	row := findingRow{
		ID:      "find-bad",
		Type:    "TIMEOUT",
		Status:  "UNCONFIRMED",
		Details: []byte(`{not json!}`),
	}
	// Should not panic
	m := row.toModel(zerolog.Nop())
	if m.ID != "find-bad" {
		t.Errorf("ID = %q, want find-bad", m.ID)
	}
}

func TestFindingRow_ToModel_EmptyDetails(t *testing.T) {
	row := findingRow{
		ID:      "find-empty",
		Type:    "TIMEOUT",
		Status:  "UNCONFIRMED",
		Details: nil,
	}
	m := row.toModel(zerolog.Nop())
	if m.ID != "find-empty" {
		t.Errorf("ID = %q, want find-empty", m.ID)
	}
}

func TestNewFindingsQuery_Empty(t *testing.T) {
	q := newFindingsQuery()
	query, args := q.build(50, 0)
	if !strings.Contains(query, "FROM findings f") {
		t.Errorf("query should contain FROM findings f, got: %s", query)
	}
	if strings.Contains(query, "WHERE") {
		t.Errorf("query should not contain WHERE with no conditions, got: %s", query)
	}
	if len(args) != 2 { // limit and offset
		t.Errorf("args len = %d, want 2", len(args))
	}
}

func TestFindingsQuery_WithCampaign(t *testing.T) {
	q := newFindingsQuery()
	q.withCampaign("camp-1")
	query, args := q.build(50, 0)
	if !strings.Contains(query, "f.campaign_id = $1") {
		t.Errorf("expected campaign filter, got: %s", query)
	}
	if len(args) != 3 { // campaign + limit + offset
		t.Errorf("args len = %d, want 3", len(args))
	}
	if args[0] != "camp-1" {
		t.Errorf("args[0] = %v, want camp-1", args[0])
	}
}

func TestFindingsQuery_WithType(t *testing.T) {
	q := newFindingsQuery()
	q.withType("TIMEOUT")
	query, args := q.build(25, 10)
	if !strings.Contains(query, "f.type = $1") {
		t.Errorf("expected type filter, got: %s", query)
	}
	if args[0] != "TIMEOUT" {
		t.Errorf("args[0] = %v, want TIMEOUT", args[0])
	}
}

func TestFindingsQuery_WithStatus(t *testing.T) {
	q := newFindingsQuery()
	q.withStatus("CONFIRMED")
	query, args := q.build(50, 0)
	if !strings.Contains(query, "f.status = $1") {
		t.Errorf("expected status filter, got: %s", query)
	}
	if args[0] != "CONFIRMED" {
		t.Errorf("args[0] = %v, want CONFIRMED", args[0])
	}
}

func TestFindingsQuery_WithSince(t *testing.T) {
	since := time.Now().Add(-time.Hour)
	q := newFindingsQuery()
	q.withSince(&since)
	query, args := q.build(50, 0)
	if !strings.Contains(query, "f.created_at >= $1") {
		t.Errorf("expected since filter, got: %s", query)
	}
	if args[0] != since {
		t.Errorf("args[0] = %v, want %v", args[0], since)
	}
}

func TestFindingsQuery_WithSince_Nil(t *testing.T) {
	q := newFindingsQuery()
	q.withSince(nil)
	query, _ := q.build(50, 0)
	if strings.Contains(query, "created_at >= ") {
		t.Errorf("nil since should not add created_at filter, got: %s", query)
	}
}

func TestFindingsQuery_EmptyFilters(t *testing.T) {
	q := newFindingsQuery()
	q.withCampaign("")
	q.withType("")
	q.withStatus("")
	q.withSince(nil)
	query, args := q.build(50, 0)
	if strings.Contains(query, "WHERE") {
		t.Errorf("empty filters should not add WHERE, got: %s", query)
	}
	if len(args) != 2 {
		t.Errorf("args len = %d, want 2", len(args))
	}
}

func TestFindingsQuery_AllFilters(t *testing.T) {
	since := time.Now().Add(-24 * time.Hour)
	q := newFindingsQuery()
	q.withCampaign("camp-1")
	q.withType("SERVER_ERROR")
	q.withStatus("CONFIRMED")
	q.withSince(&since)
	query, args := q.build(100, 50)

	if !strings.Contains(query, "WHERE") {
		t.Errorf("expected WHERE clause, got: %s", query)
	}
	if !strings.Contains(query, "f.campaign_id = $1") {
		t.Errorf("missing campaign filter, got: %s", query)
	}
	if !strings.Contains(query, "f.type = $2") {
		t.Errorf("missing type filter, got: %s", query)
	}
	if !strings.Contains(query, "f.status = $3") {
		t.Errorf("missing status filter, got: %s", query)
	}
	if !strings.Contains(query, "f.created_at >= $4") {
		t.Errorf("missing since filter, got: %s", query)
	}
	// 4 filter args + limit + offset = 6
	if len(args) != 6 {
		t.Errorf("args len = %d, want 6", len(args))
	}
	if args[4] != 100 {
		t.Errorf("limit arg = %v, want 100", args[4])
	}
	if args[5] != 50 {
		t.Errorf("offset arg = %v, want 50", args[5])
	}
}

func TestFindingsQuery_OrderByAndLimit(t *testing.T) {
	q := newFindingsQuery()
	query, _ := q.build(10, 5)
	if !strings.Contains(query, "ORDER BY f.created_at DESC") {
		t.Errorf("expected ORDER BY clause, got: %s", query)
	}
	if !strings.Contains(query, "LIMIT $1 OFFSET $2") {
		t.Errorf("expected LIMIT/OFFSET clause, got: %s", query)
	}
}
