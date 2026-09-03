package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMigrationsSortsAndRejectsDuplicates(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "002_second.sql"), []byte("SELECT 2"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "001_first.sql"), []byte("SELECT 1"), 0600); err != nil {
		t.Fatal(err)
	}
	migrations, err := LoadMigrations(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("migrations were not sorted: %#v", migrations)
	}
	if err := os.WriteFile(filepath.Join(dir, "001_duplicate.sql"), []byte("SELECT 1"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMigrations(dir); err == nil {
		t.Fatal("duplicate migration version was accepted")
	}
}

func TestMigrationsUseAnAdvisoryTransactionLock(t *testing.T) {
	if !strings.Contains(migrationLockSQL, "pg_advisory_xact_lock") {
		t.Fatal("migration lock is not transactional")
	}
}

func TestApplyMigrationsRejectsInvalidInputBeforeOpeningDatabase(t *testing.T) {
	if err := ApplyMigrations(context.Background(), nil, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing migration directory was accepted")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "002_other.sql"), []byte("SELECT 2"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(context.Background(), nil, dir); err == nil {
		t.Fatal("non-canonical migration set was accepted")
	}
}

func TestMigrationChecksumIsStable(t *testing.T) {
	first := migrationChecksum("SELECT 1")
	second := migrationChecksum("SELECT 1")
	other := migrationChecksum("SELECT 2")
	if first != second || first == other {
		t.Fatal("migration checksum is not stable")
	}
}
