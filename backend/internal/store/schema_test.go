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
