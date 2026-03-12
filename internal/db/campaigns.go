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

type CampaignStore struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

func NewCampaignStore(db *sqlx.DB, logger zerolog.Logger) *CampaignStore {
	return &CampaignStore{db: db, logger: logger}
}

type campaignRow struct {
	ID            string       `db:"id"`
	Name          string       `db:"name"`
	Status        string       `db:"status"`
	CreatedAt     time.Time    `db:"created_at"`
	UpdatedAt     time.Time    `db:"updated_at"`
	StartedAt     sql.NullTime `db:"started_at"`
	FinishedAt    sql.NullTime `db:"finished_at"`
	Config        []byte       `db:"config"`
	TestsDone     int          `db:"tests_done"`
	FindingsTotal int          `db:"findings_total"`
}

func (r campaignRow) toModel(recordingIDs []string, logger zerolog.Logger) model.Campaign {
	c := model.Campaign{
		ID:           r.ID,
		Name:         r.Name,
		Status:       model.CampaignStatus(r.Status),
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
		RecordingIDs: recordingIDs,
		ConfigJSON:   r.Config,
		TestsDone:    r.TestsDone,
		FindingsN:    r.FindingsTotal,
		Progress: &model.CampaignProgress{
			TestsDone:     r.TestsDone,
			FindingsTotal: r.FindingsTotal,
		},
	}
	if r.StartedAt.Valid {
		c.StartedAt = &r.StartedAt.Time
	}
	if r.FinishedAt.Valid {
		c.FinishedAt = &r.FinishedAt.Time
	}
	if err := json.Unmarshal(r.Config, &c.Config); err != nil {
		logger.Warn().Err(err).Str("campaign_id", r.ID).Msg("unmarshal campaign config failed")
	}
	return c
}

// Create inserts a new campaign and its recording links.
func (s *CampaignStore) Create(ctx context.Context, c model.Campaign) error {
	cfgJSON, err := json.Marshal(c.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit returns sql.ErrTxDone

	_, err = tx.ExecContext(ctx, `
		INSERT INTO campaigns (id, name, status, created_at, updated_at, config)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.Name, string(c.Status), c.CreatedAt, c.UpdatedAt, cfgJSON,
	)
	if err != nil {
		return fmt.Errorf("insert campaign: %w", err)
	}

	for _, rid := range c.RecordingIDs {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO campaign_recordings (campaign_id, recording_id) VALUES ($1, $2)`,
			c.ID, rid)
		if err != nil {
			return fmt.Errorf("link recording %s: %w", rid, err)
		}
	}

	return tx.Commit()
}

// GetByID returns a campaign with its linked recording IDs.
func (s *CampaignStore) GetByID(ctx context.Context, id string) (*model.Campaign, error) {
	var row campaignRow
	err := s.db.GetContext(ctx, &row,
		`SELECT id, name, status, created_at, updated_at, started_at, finished_at, config, tests_done, findings_total
		 FROM campaigns WHERE id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get campaign: %w", err)
	}

	rids, err := s.getRecordingIDs(ctx, id)
	if err != nil {
		return nil, err
	}

	c := row.toModel(rids, s.logger)
	return &c, nil
}

func (s *CampaignStore) getRecordingIDs(ctx context.Context, campaignID string) ([]string, error) {
	var ids []string
	err := s.db.SelectContext(ctx, &ids,
		`SELECT recording_id FROM campaign_recordings WHERE campaign_id = $1`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("get recording ids: %w", err)
	}
	return ids, nil
}

// batchGetRecordingIDs loads recording IDs for multiple campaigns in a single query.
func (s *CampaignStore) batchGetRecordingIDs(ctx context.Context, campaignIDs []string) (map[string][]string, error) {
	if len(campaignIDs) == 0 {
		return nil, nil
	}
	type row struct {
		CampaignID  string `db:"campaign_id"`
		RecordingID string `db:"recording_id"`
	}
	var rows []row
	err := s.db.SelectContext(ctx, &rows,
		`SELECT campaign_id, recording_id FROM campaign_recordings WHERE campaign_id = ANY($1)`,
		pq.Array(campaignIDs))
	if err != nil {
		return nil, fmt.Errorf("batch get recording ids: %w", err)
	}
	m := make(map[string][]string, len(campaignIDs))
	for _, r := range rows {
		m[r.CampaignID] = append(m[r.CampaignID], r.RecordingID)
	}
	return m, nil
}

// List returns campaigns with optional status filter and pagination.
func (s *CampaignStore) List(ctx context.Context, statusFilter string, limit, offset int) ([]model.Campaign, error) {
	if limit <= 0 {
		limit = 50
	}

	var rows []campaignRow
	var err error

	if statusFilter != "" {
		err = s.db.SelectContext(ctx, &rows,
			`SELECT id, name, status, created_at, updated_at, started_at, finished_at, config, tests_done, findings_total
			 FROM campaigns WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			statusFilter, limit, offset)
	} else {
		err = s.db.SelectContext(ctx, &rows,
			`SELECT id, name, status, created_at, updated_at, started_at, finished_at, config, tests_done, findings_total
			 FROM campaigns ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}

	result := make([]model.Campaign, len(rows))
	campaignIDs := make([]string, len(rows))
	for i, r := range rows {
		campaignIDs[i] = r.ID
		result[i] = r.toModel(nil, s.logger)
	}

	// Batch-load recording IDs for all listed campaigns
	recMap, err := s.batchGetRecordingIDs(ctx, campaignIDs)
	if err != nil {
		s.logger.Warn().Err(err).Msg("batch load recording ids for campaign list failed, continuing without")
	}
	if recMap != nil {
		for i := range result {
			result[i].RecordingIDs = recMap[result[i].ID]
		}
	}

	return result, nil
}

// UpdateStatus atomically transitions a campaign status. Returns false if the current
// status did not match oldStatus (optimistic lock).
func (s *CampaignStore) UpdateStatus(ctx context.Context, id string, oldStatus, newStatus model.CampaignStatus) (bool, error) {
	now := time.Now().UTC()
	var res sql.Result
	var err error

	switch newStatus {
	case model.CampaignRunning, model.CampaignStarting:
		res, err = s.db.ExecContext(ctx,
			`UPDATE campaigns SET status=$1, updated_at=$2, started_at=$3 WHERE id=$4 AND status=$5`,
			string(newStatus), now, now, id, string(oldStatus))
	case model.CampaignFinished, model.CampaignFailed, model.CampaignStopped:
		res, err = s.db.ExecContext(ctx,
			`UPDATE campaigns SET status=$1, updated_at=$2, finished_at=$3 WHERE id=$4 AND status=$5`,
			string(newStatus), now, now, id, string(oldStatus))
	default:
		res, err = s.db.ExecContext(ctx,
			`UPDATE campaigns SET status=$1, updated_at=$2 WHERE id=$3 AND status=$4`,
			string(newStatus), now, id, string(oldStatus))
	}
	if err != nil {
		return false, fmt.Errorf("update status: %w", err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		s.logger.Error().Err(raErr).Str("id", id).Msg("RowsAffected error on update campaign status")
	}
	return n > 0, nil
}

// IncrementStats atomically increments tests_done and findings_total for a campaign.
func (s *CampaignStore) IncrementStats(ctx context.Context, id string, testsDelta, findingsDelta int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE campaigns SET tests_done = tests_done + $1, findings_total = findings_total + $2, updated_at = $3 WHERE id = $4`,
		testsDelta, findingsDelta, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("increment stats: %w", err)
	}
	return nil
}

// AddRecordingsByFilter adds recordings matching the given origin and optional path prefix
// to a campaign. Duplicates are silently skipped. Returns the number of newly added recordings.
func (s *CampaignStore) AddRecordingsByFilter(ctx context.Context, campaignID, scheme, host string, port int, pathPrefix string) (int, error) {
	var res sql.Result
	var err error

	if pathPrefix != "" {
		res, err = s.db.ExecContext(ctx, `
			INSERT INTO campaign_recordings (campaign_id, recording_id)
			SELECT $1, r.id FROM recordings r
			WHERE r.target_scheme = $2 AND r.target_host = $3 AND r.target_port = $4
			  AND r.target_path LIKE $5
			ON CONFLICT (campaign_id, recording_id) DO NOTHING`,
			campaignID, scheme, host, port, pathPrefix+"%")
	} else {
		res, err = s.db.ExecContext(ctx, `
			INSERT INTO campaign_recordings (campaign_id, recording_id)
			SELECT $1, r.id FROM recordings r
			WHERE r.target_scheme = $2 AND r.target_host = $3 AND r.target_port = $4
			ON CONFLICT (campaign_id, recording_id) DO NOTHING`,
			campaignID, scheme, host, port)
	}

	if err != nil {
		return 0, fmt.Errorf("add recordings by filter: %w", err)
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}
