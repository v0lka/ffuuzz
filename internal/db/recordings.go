package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"ffuuzz/internal/endpoint"
	"ffuuzz/internal/model"
)

// RecordingStore provides PostgreSQL-backed persistence for recording sessions
// and their HTTP exchanges.
type RecordingStore struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

// NewRecordingStore creates a RecordingStore backed by the given database connection.
func NewRecordingStore(db *sqlx.DB, logger zerolog.Logger) *RecordingStore {
	return &RecordingStore{db: db, logger: logger}
}

// recordingRow is the flat DB row for the recordings table.
type recordingRow struct {
	ID            string       `db:"id"`
	SchemaVersion int          `db:"schema_version"`
	CreatedAt     sql.NullTime `db:"created_at"`
	TargetScheme  string       `db:"target_scheme"`
	TargetHost    string       `db:"target_host"`
	TargetPort    int          `db:"target_port"`
	TargetPath    string       `db:"target_path"`
	EntryCount    int          `db:"entry_count"`
}

func (r recordingRow) toModel() model.RecordingSession {
	s := model.RecordingSession{
		ID:            r.ID,
		SchemaVersion: r.SchemaVersion,
		Target: model.TargetInfo{
			Scheme: r.TargetScheme,
			Host:   r.TargetHost,
			Port:   r.TargetPort,
			Path:   r.TargetPath,
		},
		EntryCount: r.EntryCount,
	}
	if r.CreatedAt.Valid {
		s.CreatedAt = r.CreatedAt.Time
	}
	return s
}

type exchangeRow struct {
	ID            string       `db:"id"`
	RecordingID   string       `db:"recording_id"`
	RequestID     string       `db:"request_id"`
	StartedAt     sql.NullTime `db:"started_at"`
	DurationMs    int          `db:"duration_ms"`
	ReqMethod     string       `db:"req_method"`
	ReqPath       string       `db:"req_path"`
	ReqQuery      string       `db:"req_query"`
	ReqHeaders    []byte       `db:"req_headers"`
	ReqBodyB64    string       `db:"req_body_b64"`
	ReqBodyTrunc  bool         `db:"req_body_truncated"`
	RespStatus    int          `db:"resp_status"`
	RespHeaders   []byte       `db:"resp_headers"`
	RespBodyB64   string       `db:"resp_body_b64"`
	RespBodyTrunc bool         `db:"resp_body_truncated"`
	SeqOrder      int          `db:"seq_order"`
}

func (e exchangeRow) toModel(maxBodyBytes int, logger zerolog.Logger) model.Exchange {
	ex := model.Exchange{
		RequestID:  e.RequestID,
		DurationMs: int64(e.DurationMs),
		Request: model.RequestData{
			Method:        e.ReqMethod,
			Path:          e.ReqPath,
			Query:         e.ReqQuery,
			BodyB64:       e.ReqBodyB64,
			BodyTruncated: e.ReqBodyTrunc,
		},
		Response: model.ResponseData{
			Status:        e.RespStatus,
			BodyB64:       e.RespBodyB64,
			BodyTruncated: e.RespBodyTrunc,
		},
	}
	if e.StartedAt.Valid {
		ex.StartedAt = e.StartedAt.Time
	}

	if maxBodyBytes > 0 {
		// Align to 4-byte boundary for valid base64 truncation
		truncLen := (maxBodyBytes / 4) * 4
		if truncLen > 0 && len(ex.Request.BodyB64) > truncLen {
			ex.Request.BodyB64 = ex.Request.BodyB64[:truncLen]
			ex.Request.BodyTruncated = true
		}
		if truncLen > 0 && len(ex.Response.BodyB64) > truncLen {
			ex.Response.BodyB64 = ex.Response.BodyB64[:truncLen]
			ex.Response.BodyTruncated = true
		}
	}

	if len(e.ReqHeaders) > 0 {
		if err := json.Unmarshal(e.ReqHeaders, &ex.Request.Headers); err != nil {
			logger.Warn().Err(err).Str("exchange_id", e.ID).Msg("unmarshal request headers failed")
		}
	}
	if len(e.RespHeaders) > 0 {
		if err := json.Unmarshal(e.RespHeaders, &ex.Response.Headers); err != nil {
			logger.Warn().Err(err).Str("exchange_id", e.ID).Msg("unmarshal response headers failed")
		}
	}
	return ex
}

// Upsert inserts a RecordingSession. If the session ID already exists, it is skipped (idempotent).
// Returns true if inserted, false if skipped.
func (s *RecordingStore) Upsert(ctx context.Context, sess model.RecordingSession) (bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit returns sql.ErrTxDone

	// Check if already exists
	var exists bool
	err = tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM recordings WHERE id = $1)`, sess.ID)
	if err != nil {
		return false, fmt.Errorf("check exists: %w", err)
	}
	if exists {
		return false, nil
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO recordings (id, schema_version, created_at, target_scheme, target_host, target_port, target_path, entry_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		sess.ID, sess.SchemaVersion, sess.CreatedAt,
		sess.Target.Scheme, sess.Target.Host, sess.Target.Port, sess.Target.Path,
		len(sess.Entries),
	)
	if err != nil {
		return false, fmt.Errorf("insert recording: %w", err)
	}

	if err := s.insertExchanges(ctx, tx, sess.ID, sess.Entries, 0); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

// List returns recording sessions (without entries) with pagination and optional host/path filters.
func (s *RecordingStore) List(ctx context.Context, limit, offset int, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id, schema_version, created_at, target_scheme, target_host, target_port, target_path, entry_count FROM recordings`
	var conditions []string
	var args []any
	argIdx := 1

	if hostFilter != "" {
		conditions = append(conditions, fmt.Sprintf("target_host = $%d", argIdx))
		args = append(args, hostFilter)
		argIdx++
	}
	if pathPrefix != "" {
		conditions = append(conditions, fmt.Sprintf("target_path LIKE $%d ESCAPE '\\'", argIdx))
		args = append(args, pathPrefix+"%")
		argIdx++
	}

	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
	}

	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	var rows []recordingRow
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}

	result := make([]model.RecordingSession, len(rows))
	for i, r := range rows {
		result[i] = r.toModel()
	}
	return result, nil
}

// ListAll returns all recording sessions matching filters, with their entries populated.
// Unlike List, this method has no pagination and includes exchange data for export.
func (s *RecordingStore) ListAll(ctx context.Context, hostFilter, pathPrefix string) ([]model.RecordingSession, error) {
	query := `SELECT id, schema_version, created_at, target_scheme, target_host, target_port, target_path, entry_count FROM recordings`
	var conditions []string
	var args []any
	argIdx := 1

	if hostFilter != "" {
		conditions = append(conditions, fmt.Sprintf("target_host = $%d", argIdx))
		args = append(args, hostFilter)
		argIdx++
	}
	if pathPrefix != "" {
		conditions = append(conditions, fmt.Sprintf("target_path LIKE $%d ESCAPE '\\'", argIdx))
		args = append(args, pathPrefix+"%")
	}

	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
	}

	query += " ORDER BY created_at DESC"

	var rows []recordingRow
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("list all recordings: %w", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	// Collect IDs for batch exchange fetch
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}

	var exRows []exchangeRow
	exQuery, exArgs, err := sqlx.In(
		`SELECT id, recording_id, request_id, started_at, duration_ms,
			req_method, req_path, req_query, req_headers, req_body_b64, req_body_truncated,
			resp_status, resp_headers, resp_body_b64, resp_body_truncated, seq_order
		 FROM exchanges WHERE recording_id IN (?) ORDER BY recording_id, seq_order`, ids)
	if err != nil {
		return nil, fmt.Errorf("build exchange query: %w", err)
	}
	exQuery = s.db.Rebind(exQuery)
	if err := s.db.SelectContext(ctx, &exRows, exQuery, exArgs...); err != nil {
		return nil, fmt.Errorf("list all exchanges: %w", err)
	}

	// Group exchanges by recording ID
	exchangeMap := make(map[string][]model.Exchange, len(rows))
	for _, er := range exRows {
		exchangeMap[er.RecordingID] = append(exchangeMap[er.RecordingID], er.toModel(0, s.logger))
	}

	result := make([]model.RecordingSession, len(rows))
	for i, r := range rows {
		result[i] = r.toModel()
		result[i].Entries = exchangeMap[r.ID]
	}
	return result, nil
}

// GetByID returns a single recording session. If includeEntries is true, the entries are loaded.
func (s *RecordingStore) GetByID(ctx context.Context, id string, includeEntries bool, maxBodyBytes int) (*model.RecordingSession, error) {
	var row recordingRow
	err := s.db.GetContext(ctx, &row,
		`SELECT id, schema_version, created_at, target_scheme, target_host, target_port, target_path, entry_count
		 FROM recordings WHERE id = $1`, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get recording: %w", err)
	}

	sess := row.toModel()

	if includeEntries {
		var exRows []exchangeRow
		err = s.db.SelectContext(ctx, &exRows,
			`SELECT id, recording_id, request_id, started_at, duration_ms,
				req_method, req_path, req_query, req_headers, req_body_b64, req_body_truncated,
				resp_status, resp_headers, resp_body_b64, resp_body_truncated, seq_order
			 FROM exchanges WHERE recording_id = $1 ORDER BY seq_order`, id)
		if err != nil {
			return nil, fmt.Errorf("get exchanges: %w", err)
		}
		sess.Entries = make([]model.Exchange, len(exRows))
		for i, er := range exRows {
			sess.Entries[i] = er.toModel(maxBodyBytes, s.logger)
		}
	}

	return &sess, nil
}

// GetByIDs returns multiple recording sessions by their IDs.
// This is more efficient than calling GetByID in a loop.
func (s *RecordingStore) GetByIDs(ctx context.Context, ids []string) ([]model.RecordingSession, error) {
	if len(ids) == 0 {
		return []model.RecordingSession{}, nil
	}

	query, args, err := sqlx.In(
		`SELECT id, schema_version, created_at, target_scheme, target_host, target_port, target_path, entry_count
		 FROM recordings WHERE id IN (?)`, ids)
	if err != nil {
		return nil, fmt.Errorf("build query: %w", err)
	}

	query = s.db.Rebind(query)

	var rows []recordingRow
	if err := s.db.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, fmt.Errorf("get recordings by ids: %w", err)
	}

	sessions := make([]model.RecordingSession, len(rows))
	for i, row := range rows {
		sessions[i] = row.toModel()
	}

	return sessions, nil
}

// Delete removes a recording by ID. Returns true if a row was deleted.
func (s *RecordingStore) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM recordings WHERE id = $1`, id)
	if err != nil {
		return false, fmt.Errorf("delete recording: %w", err)
	}
	n, raErr := res.RowsAffected()
	if raErr != nil {
		s.logger.Error().Err(raErr).Str("id", id).Msg("RowsAffected error on delete recording")
	}
	return n > 0, nil
}

// IsUsedByActiveCampaign checks if the recording is linked to a RUNNING or STARTING campaign.
func (s *RecordingStore) IsUsedByActiveCampaign(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := s.db.GetContext(ctx, &exists,
		`SELECT EXISTS(
			SELECT 1 FROM campaign_recordings cr
			JOIN campaigns c ON c.id = cr.campaign_id
			WHERE cr.recording_id = $1 AND c.status IN ('RUNNING', 'STARTING')
		)`, id)
	if err != nil {
		return false, fmt.Errorf("check active campaign: %w", err)
	}
	return exists, nil
}

// FindOrAppend looks up an existing recording by (scheme, host, port, path).
// If found, it appends the new exchanges; otherwise it creates a new recording.
// Returns the recording ID, whether a new recording was created, and any error.
func (s *RecordingStore) FindOrAppend(ctx context.Context, sess model.RecordingSession) (string, bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rollback after commit returns sql.ErrTxDone

	// Try to find existing recording by endpoint key, lock row for update
	var existingID string
	var existingCount int
	err = tx.QueryRowxContext(ctx, `
		SELECT id, entry_count FROM recordings
		WHERE target_scheme = $1 AND target_host = $2 AND target_port = $3 AND target_path = $4
		FOR UPDATE`,
		sess.Target.Scheme, sess.Target.Host, sess.Target.Port, sess.Target.Path,
	).Scan(&existingID, &existingCount)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("find recording: %w", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		// Create new recording
		newID := uuid.New().String()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO recordings (id, schema_version, created_at, target_scheme, target_host, target_port, target_path, entry_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			newID, sess.SchemaVersion, sess.CreatedAt,
			sess.Target.Scheme, sess.Target.Host, sess.Target.Port, sess.Target.Path,
			len(sess.Entries),
		)
		if err != nil {
			return "", false, fmt.Errorf("insert recording: %w", err)
		}

		if err := s.insertExchanges(ctx, tx, newID, sess.Entries, 0); err != nil {
			return "", false, err
		}

		if err := tx.Commit(); err != nil {
			return "", false, fmt.Errorf("commit: %w", err)
		}
		return newID, true, nil
	}

	// Append to existing recording
	if err := s.insertExchanges(ctx, tx, existingID, sess.Entries, existingCount); err != nil {
		return "", false, err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE recordings SET entry_count = entry_count + $1, created_at = GREATEST(created_at, $2)
		WHERE id = $3`,
		len(sess.Entries), sess.CreatedAt, existingID,
	)
	if err != nil {
		return "", false, fmt.Errorf("update recording: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit: %w", err)
	}
	return existingID, false, nil
}

// insertExchanges inserts exchanges into a transaction with seq_order starting from seqStart.
func (s *RecordingStore) insertExchanges(ctx context.Context, tx *sqlx.Tx, recordingID string, entries []model.Exchange, seqStart int) error {
	for i, ex := range entries {
		reqHeaders, err := json.Marshal(ex.Request.Headers)
		if err != nil {
			return fmt.Errorf("marshal request headers for exchange %d: %w", i, err)
		}
		respHeaders, err := json.Marshal(ex.Response.Headers)
		if err != nil {
			return fmt.Errorf("marshal response headers for exchange %d: %w", i, err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO exchanges (recording_id, request_id, started_at, duration_ms,
				req_method, req_path, req_query, req_headers, req_body_b64, req_body_truncated,
				resp_status, resp_headers, resp_body_b64, resp_body_truncated, seq_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			recordingID, ex.RequestID, ex.StartedAt, ex.DurationMs,
			ex.Request.Method, ex.Request.Path, ex.Request.Query,
			reqHeaders, ex.Request.BodyB64, ex.Request.BodyTruncated,
			ex.Response.Status,
			respHeaders, ex.Response.BodyB64, ex.Response.BodyTruncated,
			seqStart+i,
		)
		if err != nil {
			return fmt.Errorf("insert exchange %d: %w", i, err)
		}
	}
	return nil
}

// GetTree returns aggregated recording counts grouped by endpoint for building the tree view.
func (s *RecordingStore) GetTree(ctx context.Context) ([]model.TreeEntry, error) {
	var entries []model.TreeEntry
	err := s.db.SelectContext(ctx, &entries,
		`SELECT target_scheme, target_host, target_port, target_path, COUNT(*) as cnt
		 FROM recordings
		 GROUP BY target_scheme, target_host, target_port, target_path
		 ORDER BY target_scheme, target_host, target_port, target_path`)
	if err != nil {
		return nil, fmt.Errorf("get tree: %w", err)
	}
	return entries, nil
}

// ErrRecordingsInUse is returned when attempting to delete recordings used by active campaigns.
var ErrRecordingsInUse = errors.New("recordings are used by active campaigns")

// DeleteByPrefix deletes recordings matching the given origin and path prefix.
// Returns the number of deleted recordings. Returns ErrRecordingsInUse if any
// matching recording is linked to a RUNNING or STARTING campaign.
func (s *RecordingStore) DeleteByPrefix(ctx context.Context, scheme, host string, port int, pathPrefix string) (int64, error) {
	// Build the WHERE clause for matching recordings
	baseWhere := `target_scheme = $1 AND target_host = $2 AND target_port = $3`
	args := []any{scheme, host, port}
	if pathPrefix != "" {
		baseWhere += ` AND target_path LIKE $4 ESCAPE '\'`
		args = append(args, pathPrefix+"%")
	}

	// Check if any matching recordings are used by active campaigns
	var inUse bool
	err := s.db.GetContext(ctx, &inUse, fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM recordings r
			JOIN campaign_recordings cr ON cr.recording_id = r.id
			JOIN campaigns c ON c.id = cr.campaign_id
			WHERE %s AND c.status IN ('RUNNING', 'STARTING')
		)`, baseWhere), args...)
	if err != nil {
		return 0, fmt.Errorf("check active campaigns: %w", err)
	}
	if inUse {
		return 0, ErrRecordingsInUse
	}

	res, err := s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM recordings WHERE %s`, baseWhere), args...)
	if err != nil {
		return 0, fmt.Errorf("delete by prefix: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		s.logger.Warn().Err(err).Msg("rows affected unavailable for delete by prefix")
	}
	return n, nil
}

// ListOrigins returns all distinct (scheme, host, port) tuples in the
// recordings table. Implements endpoint.Merger.
func (s *RecordingStore) ListOrigins(ctx context.Context) ([]endpoint.Origin, error) {
	rows, err := s.db.QueryxContext(ctx,
		`SELECT DISTINCT target_scheme, target_host, target_port FROM recordings`)
	if err != nil {
		return nil, fmt.Errorf("list origins: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var origins []endpoint.Origin
	for rows.Next() {
		var o endpoint.Origin
		if err := rows.Scan(&o.Scheme, &o.Host, &o.Port); err != nil {
			return nil, fmt.Errorf("scan origin: %w", err)
		}
		origins = append(origins, o)
	}
	return origins, rows.Err()
}

// ListDistinctPaths returns all distinct target_path values for a given
// origin. Implements endpoint.Merger.
func (s *RecordingStore) ListDistinctPaths(ctx context.Context, origin endpoint.Origin) ([]string, error) {
	var paths []string
	err := s.db.SelectContext(ctx, &paths,
		`SELECT DISTINCT target_path FROM recordings
		 WHERE target_scheme = $1 AND target_host = $2 AND target_port = $3`,
		origin.Scheme, origin.Host, origin.Port)
	if err != nil {
		return nil, fmt.Errorf("list distinct paths: %w", err)
	}
	return paths, nil
}

// MergeRecordings handles the DB side of an endpoint collapse. For every
// recording whose target_path starts with one of the sourcePrefixes, the path
// is rewritten so that the collapsed segment becomes {_}. When multiple
// recordings end up with the same rewritten path they are physically merged
// (exchanges are moved, duplicates deleted). Recordings linked to active
// campaigns are skipped. Implements endpoint.Merger.
func (s *RecordingStore) MergeRecordings(ctx context.Context, origin endpoint.Origin, sourcePrefixes []string, targetPrefix string) (int, error) {
	if len(sourcePrefixes) == 0 {
		return 0, nil
	}

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin merge tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Step 1: Collect affected recordings (not linked to active campaigns).
	type affected struct {
		ID   string `db:"id"`
		Path string `db:"target_path"`
	}

	// Build LIKE conditions for all source prefixes.
	conditions := make([]string, 0, len(sourcePrefixes))
	args := []any{origin.Scheme, origin.Host, origin.Port}
	for i, sp := range sourcePrefixes {
		conditions = append(conditions, fmt.Sprintf("r.target_path LIKE $%d ESCAPE '\\'", 4+i))
		args = append(args, sp+"%")
	}

	query := fmt.Sprintf(`
		SELECT r.id, r.target_path FROM recordings r
		WHERE r.target_scheme = $1 AND r.target_host = $2 AND r.target_port = $3
		  AND (%s)
		  AND NOT EXISTS (
		      SELECT 1 FROM campaign_recordings cr
		      JOIN campaigns c ON c.id = cr.campaign_id
		      WHERE cr.recording_id = r.id AND c.status IN ('RUNNING', 'STARTING')
		  )
		FOR UPDATE OF r`, strings.Join(conditions, " OR "))

	var affectedRows []affected
	if err := tx.SelectContext(ctx, &affectedRows, query, args...); err != nil {
		return 0, fmt.Errorf("select affected recordings: %w", err)
	}

	if len(affectedRows) == 0 {
		return 0, nil
	}

	// Step 2: Compute new path for each recording and group by new path.
	type pathGroup struct {
		newPath string
		ids     []string
	}
	groups := make(map[string]*pathGroup)
	for _, r := range affectedRows {
		newPath := rewriteTargetPath(r.Path, sourcePrefixes, targetPrefix)
		g, ok := groups[newPath]
		if !ok {
			g = &pathGroup{newPath: newPath}
			groups[newPath] = g
		}
		g.ids = append(g.ids, r.ID)
	}

	merged := 0

	// Step 3: For each group, merge into a single recording.
	for _, g := range groups {
		// Check if a recording already exists at the new path (e.g. from
		// heuristic normalisation).
		var existingID string
		err := tx.GetContext(ctx, &existingID, `
			SELECT id FROM recordings
			WHERE target_scheme = $1 AND target_host = $2 AND target_port = $3 AND target_path = $4
			FOR UPDATE`,
			origin.Scheme, origin.Host, origin.Port, g.newPath)

		keeperID := ""
		if err == nil {
			// Existing recording at new path — use it as the keeper.
			keeperID = existingID
		} else if errors.Is(err, sql.ErrNoRows) {
			// No existing recording. Use the first affected recording as the
			// keeper and update its path.
			keeperID = g.ids[0]
			g.ids = g.ids[1:]

			if _, err = tx.ExecContext(ctx, `
				UPDATE recordings SET target_path = $1 WHERE id = $2`,
				g.newPath, keeperID); err != nil {
				return 0, fmt.Errorf("update keeper path: %w", err)
			}
		} else {
			return 0, fmt.Errorf("check existing recording: %w", err)
		}

		// Move exchanges from non-keeper recordings to the keeper.
		for _, srcID := range g.ids {
			if srcID == keeperID {
				continue
			}

			// Get current max seq_order on the keeper.
			var maxSeq sql.NullInt64
			if err = tx.GetContext(ctx, &maxSeq, `
				SELECT MAX(seq_order) FROM exchanges WHERE recording_id = $1`, keeperID); err != nil {
				return 0, fmt.Errorf("get max seq: %w", err)
			}
			base := 0
			if maxSeq.Valid {
				base = int(maxSeq.Int64) + 1
			}

			// Renumber and move exchanges.
			if _, err = tx.ExecContext(ctx, `
				UPDATE exchanges
				SET recording_id = $1,
				    seq_order = seq_order + $2
				WHERE recording_id = $3`,
				keeperID, base, srcID); err != nil {
				return 0, fmt.Errorf("move exchanges: %w", err)
			}

			// Delete the now-empty source recording.
			if _, err = tx.ExecContext(ctx, `DELETE FROM recordings WHERE id = $1`, srcID); err != nil {
				return 0, fmt.Errorf("delete merged recording: %w", err)
			}

			merged++
		}

		// Update keeper's entry_count.
		if _, err = tx.ExecContext(ctx, `
			UPDATE recordings SET entry_count = (
				SELECT COUNT(*) FROM exchanges WHERE recording_id = $1
			) WHERE id = $1`, keeperID); err != nil {
			return 0, fmt.Errorf("update entry count: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit merge: %w", err)
	}

	return merged, nil
}

// rewriteTargetPath replaces the first matching source prefix in path with the
// target prefix. E.g. path="/users/alice/posts", source="/users/alice",
// target="/users/{_}" → "/users/{_}/posts".
func rewriteTargetPath(path string, sourcePrefixes []string, targetPrefix string) string {
	for _, sp := range sourcePrefixes {
		if strings.HasPrefix(path, sp) {
			return targetPrefix + path[len(sp):]
		}
	}
	return path
}
