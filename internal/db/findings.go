package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

// FindingStore provides PostgreSQL-backed persistence for fuzzing findings
// with flexible filtering and pagination.
type FindingStore struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

// NewFindingStore creates a FindingStore backed by the given database connection.
func NewFindingStore(db *sqlx.DB, logger zerolog.Logger) *FindingStore {
	return &FindingStore{db: db, logger: logger}
}

type findingRow struct {
	ID                  string         `db:"id"`
	CampaignID          string         `db:"campaign_id"`
	Type                string         `db:"type"`
	Status              string         `db:"status"`
	Signature           string         `db:"signature"`
	CreatedAt           time.Time      `db:"created_at"`
	ConfirmedAt         sql.NullTime   `db:"confirmed_at"`
	Method              string         `db:"method"`
	Endpoint            string         `db:"endpoint"`
	Details             []byte         `db:"details"`
	SeedRecordingID     sql.NullString `db:"seed_recording_id"`
	Minimized           bool           `db:"minimized"`
	ReproduceStatus     sql.NullString `db:"reproduce_status"`
	ReproduceEnqueuedAt sql.NullTime   `db:"reproduce_enqueued_at"`
	ReproduceRuns       int            `db:"reproduce_runs"`
	MutationType        sql.NullString `db:"mutation_type"`
	MutationPayload     sql.NullString `db:"mutation_payload"`
}

func (r findingRow) toModel(logger zerolog.Logger) model.Finding {
	f := model.Finding{
		ID:          r.ID,
		CampaignID:  r.CampaignID,
		Type:        model.FindingType(r.Type),
		Status:      model.FindingStatus(r.Status),
		Signature:   r.Signature,
		CreatedAt:   r.CreatedAt,
		Method:      r.Method,
		Endpoint:    r.Endpoint,
		Minimized:   r.Minimized,
		DetailsJSON: r.Details,
	}
	if r.ConfirmedAt.Valid {
		f.ConfirmedAt = &r.ConfirmedAt.Time
	}
	if r.SeedRecordingID.Valid {
		f.SeedRecordingID = r.SeedRecordingID.String
	}
	if r.ReproduceStatus.Valid {
		f.ReproduceStatus = r.ReproduceStatus.String
	}
	if r.ReproduceEnqueuedAt.Valid {
		f.ReproduceEnqueuedAt = &r.ReproduceEnqueuedAt.Time
	}
	f.ReproduceRuns = r.ReproduceRuns
	if r.MutationType.Valid {
		f.MutationType = r.MutationType.String
	}
	if r.MutationPayload.Valid {
		f.MutationPayload = r.MutationPayload.String
	}
	if len(r.Details) > 0 {
		if err := json.Unmarshal(r.Details, &f.Details); err != nil {
			logger.Warn().Err(err).Str("finding_id", r.ID).Msg("unmarshal finding details failed")
		}
	}
	return f
}

// Create inserts a new finding.
func (s *FindingStore) Create(ctx context.Context, f model.Finding) error {
	detailsJSON, err := json.Marshal(f.Details)
	if err != nil {
		return fmt.Errorf("marshal details: %w", err)
	}

	var seedRec sql.NullString
	if f.SeedRecordingID != "" {
		seedRec = sql.NullString{String: f.SeedRecordingID, Valid: true}
	}

	var mutationType, mutationPayload sql.NullString
	if f.MutationType != "" {
		mutationType = sql.NullString{String: f.MutationType, Valid: true}
	}
	if f.MutationPayload != "" {
		mutationPayload = sql.NullString{String: f.MutationPayload, Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO findings (id, campaign_id, type, status, signature, created_at, method, endpoint, details, seed_recording_id, minimized, mutation_type, mutation_payload)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		f.ID, f.CampaignID, string(f.Type), string(f.Status), f.Signature,
		f.CreatedAt, f.Method, f.Endpoint, detailsJSON, seedRec, f.Minimized, mutationType, mutationPayload,
	)
	if err != nil {
		return fmt.Errorf("insert finding: %w", err)
	}
	return nil
}

// loadArtifactID returns the artifact ID for a finding, or empty string if none.
func (s *FindingStore) loadArtifactID(ctx context.Context, findingID string) string {
	var artifactID sql.NullString
	if err := s.db.GetContext(ctx, &artifactID,
		`SELECT id FROM artifacts WHERE finding_id = $1 LIMIT 1`, findingID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.logger.Warn().Err(err).Str("finding_id", findingID).Msg("load artifact ID failed")
	}
	if artifactID.Valid {
		return artifactID.String
	}
	return ""
}

// batchLoadArtifactIDs fetches artifact IDs for multiple findings in a single query.
func (s *FindingStore) batchLoadArtifactIDs(ctx context.Context, findingIDs []string) (map[string]string, error) {
	if len(findingIDs) == 0 {
		return nil, nil
	}
	type row struct {
		FindingID  string `db:"finding_id"`
		ArtifactID string `db:"id"`
	}
	var rows []row
	err := s.db.SelectContext(ctx, &rows,
		`SELECT DISTINCT ON (finding_id) finding_id, id FROM artifacts WHERE finding_id = ANY($1)`,
		pq.Array(findingIDs))
	if err != nil {
		return nil, fmt.Errorf("batch load artifact ids: %w", err)
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.FindingID] = r.ArtifactID
	}
	return m, nil
}

// GetByID returns a single finding by ID.
func (s *FindingStore) GetByID(ctx context.Context, id string) (*model.Finding, error) {
	var row findingRow
	err := s.db.GetContext(ctx, &row,
		`SELECT id, campaign_id, type, status, signature, created_at, confirmed_at,
			method, endpoint, details, seed_recording_id, minimized,
			reproduce_status, reproduce_enqueued_at, reproduce_runs,
			mutation_type, mutation_payload
		 FROM findings WHERE id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get finding: %w", err)
	}
	f := row.toModel(s.logger)
	f.ArtifactID = s.loadArtifactID(ctx, id)

	return &f, nil
}

// findingsQuery builds dynamic SQL for listing findings with optional filters.
type findingsQuery struct {
	conditions []string
	args       []any
	argIdx     int
}

func newFindingsQuery() *findingsQuery {
	return &findingsQuery{argIdx: 1}
}

func (q *findingsQuery) withCampaign(id string) {
	if id != "" {
		q.conditions = append(q.conditions, fmt.Sprintf("f.campaign_id = $%d", q.argIdx))
		q.args = append(q.args, id)
		q.argIdx++
	}
}

func (q *findingsQuery) withType(t string) {
	if t != "" {
		q.conditions = append(q.conditions, fmt.Sprintf("f.type = $%d", q.argIdx))
		q.args = append(q.args, t)
		q.argIdx++
	}
}

func (q *findingsQuery) withStatus(s string) {
	if s != "" {
		q.conditions = append(q.conditions, fmt.Sprintf("f.status = $%d", q.argIdx))
		q.args = append(q.args, s)
		q.argIdx++
	}
}

func (q *findingsQuery) withSince(t *time.Time) {
	if t != nil {
		q.conditions = append(q.conditions, fmt.Sprintf("f.created_at >= $%d", q.argIdx))
		q.args = append(q.args, *t)
		q.argIdx++
	}
}

func (q *findingsQuery) build(limit, offset int) (string, []any) {
	query := `SELECT f.id, f.campaign_id, f.type, f.status, f.signature, f.created_at, f.confirmed_at,
		f.method, f.endpoint, f.details, f.seed_recording_id, f.minimized,
		f.reproduce_status, f.reproduce_enqueued_at, f.reproduce_runs,
		f.mutation_type, f.mutation_payload
		FROM findings f`
	if len(q.conditions) > 0 {
		query += " WHERE "
		for i, cond := range q.conditions {
			if i > 0 {
				query += " AND "
			}
			query += cond
		}
	}
	query += fmt.Sprintf(" ORDER BY f.created_at DESC LIMIT $%d OFFSET $%d", q.argIdx, q.argIdx+1)
	q.args = append(q.args, limit, offset)
	return query, q.args
}

// listFindings executes a findings query and batch-loads artifact IDs.
func (s *FindingStore) listFindings(ctx context.Context, query string, args []any) ([]model.Finding, error) {
	var rows []findingRow
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	result := make([]model.Finding, len(rows))
	findingIDs := make([]string, len(rows))
	for i, r := range rows {
		result[i] = r.toModel(s.logger)
		findingIDs[i] = r.ID
	}

	artifactMap, err := s.batchLoadArtifactIDs(ctx, findingIDs)
	if err != nil {
		s.logger.Warn().Err(err).Msg("batch load artifact ids failed, continuing without")
	}
	if artifactMap != nil {
		for i, f := range result {
			if aid, ok := artifactMap[f.ID]; ok {
				result[i].ArtifactID = aid
			}
		}
	}

	return result, nil
}

// ExistsBySignature checks if a finding with the given signature already exists in this campaign.
func (s *FindingStore) ExistsBySignature(ctx context.Context, campaignID, signature string) (bool, error) {
	var exists bool
	err := s.db.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM findings WHERE campaign_id = $1 AND signature = $2)`,
		campaignID, signature)
	if err != nil {
		return false, fmt.Errorf("check signature: %w", err)
	}
	return exists, nil
}

// UpdateReproduceStatus updates the reproduce_status, enqueued_at time, and runs count.
func (s *FindingStore) UpdateReproduceStatus(ctx context.Context, id, status string, runs int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE findings SET reproduce_status = $1, reproduce_enqueued_at = $2, reproduce_runs = $3 WHERE id = $4`,
		status, time.Now().UTC(), runs, id)
	if err != nil {
		return fmt.Errorf("update reproduce status: %w", err)
	}
	return nil
}

// SetReproduceStatus updates only the reproduce_status field (used by the reproduce worker).
func (s *FindingStore) SetReproduceStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE findings SET reproduce_status = $1 WHERE id = $2`,
		status, id)
	if err != nil {
		return fmt.Errorf("set reproduce status: %w", err)
	}
	return nil
}

// ClaimNextReproduceJob atomically claims the oldest ENQUEUED finding for reproduction.
// Returns the finding ID, requested runs, and whether a job was found.
func (s *FindingStore) ClaimNextReproduceJob(ctx context.Context) (string, int, bool, error) {
	type claimResult struct {
		ID            string `db:"id"`
		ReproduceRuns int    `db:"reproduce_runs"`
	}
	var result claimResult
	err := s.db.GetContext(ctx, &result,
		`UPDATE findings
		 SET reproduce_status = $1
		 WHERE id = (
			SELECT id FROM findings
			WHERE reproduce_status = $2
			ORDER BY reproduce_enqueued_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, reproduce_runs`,
		string(model.ReproduceRunning), string(model.ReproduceEnqueued))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", 0, false, nil
		}
		return "", 0, false, fmt.Errorf("claim reproduce job: %w", err)
	}
	return result.ID, result.ReproduceRuns, true, nil
}

// UpdateStatus updates a finding's status and optionally confirmed_at.
func (s *FindingStore) UpdateStatus(ctx context.Context, id string, status model.FindingStatus) error {
	if status == model.FindingConfirmed {
		now := time.Now().UTC()
		_, err := s.db.ExecContext(ctx,
			`UPDATE findings SET status = $1, confirmed_at = $2 WHERE id = $3`,
			string(status), now, id)
		if err != nil {
			return fmt.Errorf("update finding status: %w", err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE findings SET status = $1 WHERE id = $2`, string(status), id)
	if err != nil {
		return fmt.Errorf("update finding status: %w", err)
	}
	return nil
}

// ListAll returns findings across all campaigns with optional filters and pagination.
func (s *FindingStore) ListAll(ctx context.Context, campaignID, typeFilter, statusFilter string, since *time.Time, limit, offset int) ([]model.Finding, error) {
	if limit <= 0 {
		limit = 50
	}

	q := newFindingsQuery()
	q.withCampaign(campaignID)
	q.withType(typeFilter)
	q.withStatus(statusFilter)
	q.withSince(since)
	query, args := q.build(limit, offset)

	findings, err := s.listFindings(ctx, query, args)
	if err != nil {
		return nil, fmt.Errorf("list all findings: %w", err)
	}
	return findings, nil
}

// CountByType returns the number of findings grouped by type for a campaign.
func (s *FindingStore) CountByType(ctx context.Context, campaignID string) (map[model.FindingType]int, error) {
	type row struct {
		Type  string `db:"type"`
		Count int    `db:"cnt"`
	}
	var rows []row
	err := s.db.SelectContext(ctx, &rows,
		`SELECT type, COUNT(*) AS cnt FROM findings WHERE campaign_id = $1 GROUP BY type`,
		campaignID)
	if err != nil {
		return nil, fmt.Errorf("count findings by type: %w", err)
	}
	m := make(map[model.FindingType]int, len(rows))
	for _, r := range rows {
		m[model.FindingType(r.Type)] = r.Count
	}
	return m, nil
}
