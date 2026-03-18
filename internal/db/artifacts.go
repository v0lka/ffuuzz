package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"ffuuzz/internal/model"
)

type ArtifactStore struct {
	db     *sqlx.DB
	logger zerolog.Logger
}

func NewArtifactStore(db *sqlx.DB, logger zerolog.Logger) *ArtifactStore {
	return &ArtifactStore{db: db, logger: logger}
}

// Create inserts a new artifact record.
func (s *ArtifactStore) Create(ctx context.Context, a model.Artifact) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO artifacts (id, finding_id, file_path, created_at, size_bytes)
		VALUES ($1, $2, $3, $4, $5)`,
		a.ID, a.FindingID, a.FilePath, a.CreatedAt, a.SizeBytes,
	)
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	return nil
}

// GetByFindingID returns the artifact for a given finding.
func (s *ArtifactStore) GetByFindingID(ctx context.Context, findingID string) (*model.Artifact, error) {
	var a model.Artifact
	err := s.db.GetContext(ctx, &a,
		`SELECT id, finding_id, file_path, created_at, size_bytes
		 FROM artifacts WHERE finding_id = $1 LIMIT 1`, findingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get artifact: %w", err)
	}
	return &a, nil
}
