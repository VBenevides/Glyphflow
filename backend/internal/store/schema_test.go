package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func migrationSQL(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "migrations", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func assertMigrationContains(t *testing.T, name string, fragments ...string) {
	t.Helper()
	sql := migrationSQL(t, name)
	for _, fragment := range fragments {
		if !strings.Contains(sql, fragment) {
			t.Errorf("%s does not contain %q", name, fragment)
		}
	}
}

func TestTaskDefinitionsMigration(t *testing.T) {
	assertMigrationContains(t, "002_task_definitions.sql", "CREATE TABLE task_definitions", "schedule", "timezone", "command jsonb", "retry_policy")
}

func TestTaskRunsMigration(t *testing.T) {
	assertMigrationContains(t, "003_task_runs.sql", "CREATE TABLE task_runs", "task_definition_id", "occurrence_at", "runner_id", "state", "attempt", "lease_token", "state_version")
}

func TestUniqueTaskOccurrencesMigration(t *testing.T) {
	assertMigrationContains(t, "004_unique_task_occurrences.sql", "CREATE UNIQUE INDEX", "task_definition_id, occurrence_at")
}

func TestRunEventsMigration(t *testing.T) {
	assertMigrationContains(t, "005_run_events.sql", "CREATE TABLE run_events", "event_id", "task_run_id", "sequence", "payload jsonb", "append-only")
}

func TestUniqueRunEventsMigration(t *testing.T) {
	assertMigrationContains(t, "006_unique_run_events.sql", "CREATE UNIQUE INDEX", "task_run_id, attempt, sequence")
	if !strings.Contains(migrationSQL(t, "005_run_events.sql"), "event_id text PRIMARY KEY") {
		t.Fatal("run_events does not enforce unique event IDs")
	}
}

func TestRunnersMigration(t *testing.T) {
	assertMigrationContains(t, "007_runners.sql", "CREATE TABLE runners", "pool", "capacity", "capabilities jsonb", "state", "heartbeat_at")
}

func TestRunnerKeysMigration(t *testing.T) {
	assertMigrationContains(t, "008_runner_keys.sql", "CREATE TABLE runner_keys", "public_key bytea", "not_before", "not_after", "revoked_at", "REFERENCES runners")
}
