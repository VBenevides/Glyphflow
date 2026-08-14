package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV1MigrationContainsIdentityAndVersionConstraints(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "017_v1_identity_and_execution.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, fragment := range []string{"users_v1", "user_passwords_v1", "auth_sessions_v1", "roles_v1", "permissions_v1", "sso_providers_v1", "sso_authorization_states_v1", "audit_events_v1", "unique (provider_id, provider_subject)"} {
		if !strings.Contains(sql, strings.ToLower(fragment)) {
			t.Fatalf("v1 migration missing %q", fragment)
		}
	}
}

func TestTaskVersionsV1IsDefinedOnce(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		count += strings.Count(strings.ToLower(string(raw)), "create table if not exists task_versions_v1")
	}
	if count != 1 {
		t.Fatalf("task_versions_v1 is defined %d times, want once", count)
	}
}

func TestRevision3MigrationContainsExecutionIntegrityConstraints(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "018_v1_execution_revision3.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, fragment := range []string{
		"runner_pools_v1", "runner_sessions_v1_active_idx", "tasks_v1_current_version_fk", "task_versions_v1",
		"schedule_versions_v1", "runs_v1_schedule_occurrence_idx", "execution_attempts_v1",
		"resource_leases_v1_active_idx", "dispatch_outbox_v1", "event_inbox_v1",
		"unique (execution_attempt_id, state_sequence)", "unique (execution_attempt_id, stream, chunk_sequence)",
	} {
		if !strings.Contains(sql, strings.ToLower(fragment)) {
			t.Fatalf("revision 3 migration missing %q", fragment)
		}
	}
}
