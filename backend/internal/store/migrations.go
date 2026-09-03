package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

const migrationLockSQL = `SELECT pg_advisory_xact_lock(hashtextextended('glyphflow:migrations', 0))`

func LoadMigrations(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var migrations []Migration
	seen := make(map[int]bool)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		parts := strings.SplitN(strings.TrimSuffix(entry.Name(), ".sql"), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("migration %q must be VERSION_NAME.sql", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("migration %q has invalid version", entry.Name())
		}
		if seen[version] {
			return nil, fmt.Errorf("duplicate migration version %d", version)
		}
		seen[version] = true
		sql, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, Migration{Version: version, Name: parts[1], SQL: string(sql)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	return migrations, nil
}

func ApplyMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	migrations, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "canonical" {
		return errors.New("exactly one canonical migration is required")
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version integer PRIMARY KEY,
		name text NOT NULL,
		checksum text NOT NULL DEFAULT '',
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	for _, migration := range migrations {
		if err := applyMigration(ctx, pool, migration); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, pool *pgxpool.Pool, migration Migration) error {
	checksum := migrationChecksum(migration.SQL)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	if _, err = tx.Exec(ctx, migrationLockSQL); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("lock migrations: %w", err)
	}
	var storedChecksum string
	err = tx.QueryRow(ctx, `SELECT checksum FROM schema_migrations WHERE version = $1`, migration.Version).Scan(&storedChecksum)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("check migration %d: %w", migration.Version, err)
	}
	if err == nil {
		if storedChecksum != checksum {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %d checksum changed", migration.Version)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d checksum: %w", migration.Version, err)
		}
		return nil
	}
	if _, err = tx.Exec(ctx, migration.SQL); err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO schema_migrations (version, name, checksum) VALUES ($1, $2, $3)`, migration.Version, migration.Name, checksum)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("apply migration %d: %w", migration.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}

func migrationChecksum(sql string) string {
	digest := sha256.Sum256([]byte(sql))
	return hex.EncodeToString(digest[:])
}
