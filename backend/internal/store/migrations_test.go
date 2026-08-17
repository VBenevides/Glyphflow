package store

import (
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

func TestMigrationChecksumIsStable(t *testing.T) {
	if migrationChecksum("SELECT 1") != migrationChecksum("SELECT 1") || migrationChecksum("SELECT 1") == migrationChecksum("SELECT 2") {
		t.Fatal("migration checksum is not stable")
	}
}

func TestRunLifecycleMigrationAddsDispatchedAndSystemCodes(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var lifecycle Migration
	for _, migration := range migrations {
		if migration.Name == "run_lifecycle" {
			lifecycle = migration
		}
	}
	sql := strings.ToLower(lifecycle.SQL)
	for _, fragment := range []string{"runs_state_check", "'dispatched'", "(5, 'start failure'", "(6, 'timeout'"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("run lifecycle migration does not contain %q", fragment)
		}
	}
}
