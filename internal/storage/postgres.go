package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"t-invest-bot/internal/config"
)

func Open(ctx context.Context, cfg config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.ConnectionString())
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

func RequireMigration(ctx context.Context, db *sql.DB, required int64) error {
	var checksum string
	err := db.QueryRowContext(ctx, `
		SELECT checksum
		FROM schema_migrations
		WHERE version = $1
	`, required).Scan(&checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("required database migration %d is not applied", required)
	}
	if err != nil {
		return fmt.Errorf("verify database migration %d: %w", required, err)
	}
	if checksum == "" {
		return fmt.Errorf("verify database migration %d: empty checksum", required)
	}
	return nil
}
