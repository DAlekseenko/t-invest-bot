package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

var migrationName = regexp.MustCompile(`^(\d+)_([a-z0-9][a-z0-9_-]*)\.up\.sql$`)

type migration struct {
	version  int64
	name     string
	contents []byte
	checksum string
}

func Apply(ctx context.Context, db *sql.DB, files fs.FS) error {
	if err := ensureTable(ctx, db); err != nil {
		return err
	}

	migrations, err := load(files)
	if err != nil {
		return err
	}
	for _, item := range migrations {
		if err := applyOne(ctx, db, item); err != nil {
			return err
		}
	}
	return nil
}

func ensureTable(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			checksum CHAR(64) NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func load(files fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}

	var result []migration
	seen := make(map[int64]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationName.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("parse migration version in %q", entry.Name())
		}
		if previous, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %d in %q and %q", version, previous, entry.Name())
		}
		contents, err := fs.ReadFile(files, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if len(contents) == 0 {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		sum := sha256.Sum256(contents)
		result = append(result, migration{
			version:  version,
			name:     filepath.Base(entry.Name()),
			contents: contents,
			checksum: hex.EncodeToString(sum[:]),
		})
		seen[version] = entry.Name()
	}
	if len(result) == 0 {
		return nil, errors.New("no up migrations found")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	return result, nil
}

func applyOne(ctx context.Context, db *sql.DB, item migration) error {
	var existingChecksum string
	err := db.QueryRowContext(ctx, `
		SELECT checksum FROM schema_migrations WHERE version = $1
	`, item.version).Scan(&existingChecksum)
	switch {
	case err == nil:
		if existingChecksum != item.checksum {
			return fmt.Errorf("migration %d was modified after application", item.version)
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("check migration %d: %w", item.version, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", item.version, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, string(item.contents)); err != nil {
		return fmt.Errorf("apply migration %d: %w", item.version, err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO schema_migrations (version, name, checksum)
		VALUES ($1, $2, $3)
	`, item.version, item.name, item.checksum); err != nil {
		return fmt.Errorf("record migration %d: %w", item.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", item.version, err)
	}
	return nil
}
