// Package db provides PostgreSQL-backed storage for recordings, campaigns, findings, and artifacts.
package db

import (
	"context"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Database wraps a sqlx.DB connection and provides migration support.
type Database struct {
	DB     *sqlx.DB
	logger zerolog.Logger
}

// Open connects to PostgreSQL and runs pending migrations.
func Open(dsn string, logger zerolog.Logger) (*Database, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	d := &Database{DB: db, logger: logger}

	if err := d.runMigrations(dsn); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return d, nil
}

func (d *Database) runMigrations(dsn string) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}

	d.logger.Info().Msg("database migrations applied")
	return nil
}

// Ping checks database connectivity.
func (d *Database) Ping(ctx context.Context) error {
	return d.DB.PingContext(ctx)
}

// Close closes the database connection.
func (d *Database) Close() error {
	return d.DB.Close()
}
