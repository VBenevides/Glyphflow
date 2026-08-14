package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canonicalMigrationSQL(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_canonical.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ToLower(string(raw))
}

func TestCanonicalMigrationIsTheOnlyMigration(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "canonical" {
		t.Fatalf("migrations = %#v, want only 001_canonical.sql", migrations)
	}
	if strings.Contains(migrations[0].SQL, "_v1") || strings.Contains(migrations[0].SQL, "_v2") || strings.Contains(migrations[0].SQL, "_vx") {
		t.Fatal("canonical migration contains a version-suffixed identifier")
	}
}

func TestCanonicalMigrationContainsTargetTables(t *testing.T) {
	sql := canonicalMigrationSQL(t)
	for _, table := range []string{
		"config", "users", "user_passwords", "auth_sessions", "roles", "permissions", "role_permissions", "role_assignments",
		"sso_providers", "sso_group_role_mappings", "user_sso_identities", "sso_authorization_states", "audit_events",
		"runner_pools", "runners", "runner_sessions", "runner_keys", "runner_enrollments", "tasks", "task_versions",
		"schedules", "schedule_versions", "runs", "execution_attempts", "run_events", "execution_log_chunks", "resources",
		"task_resource_requirements", "resource_leases", "dispatch_outbox", "event_inbox",
	} {
		if !strings.Contains(sql, "create table "+table+" ") {
			t.Errorf("canonical migration does not define %s", table)
		}
	}
}

func TestCanonicalMigrationHasIntegrityGuards(t *testing.T) {
	sql := canonicalMigrationSQL(t)
	for _, fragment := range []string{
		"foreign key (current_version_id, id) references task_versions",
		"unique index runner_sessions_active_idx",
		"unique index resource_leases_active_idx",
		"unique (execution_attempt_id, state_sequence)",
		"unique (execution_attempt_id, stream, chunk_sequence)",
		"unique index runs_schedule_occurrence_idx",
		"unique index runner_enrollments_unused_idx",
		"create trigger task_versions_immutable",
		"create trigger schedule_versions_immutable",
		"create trigger audit_events_append_only",
		"create trigger run_events_append_only",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("canonical migration does not contain %q", fragment)
		}
	}
}

func TestCanonicalMigrationHasIdentityConstraints(t *testing.T) {
	sql := canonicalMigrationSQL(t)
	for _, fragment := range []string{
		"check (email <> '' and email = lower(btrim(email)))",
		"check (username <> '' and username = lower(btrim(username)))",
		"password_hash ~ '^\\$argon2id\\$v=19\\$m=",
		"unique index roles_name_ci_idx on roles (lower(name))",
		"create trigger roles_system_immutable",
		"role_id text not null references roles(id) on delete restrict",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("canonical migration does not contain identity constraint %q", fragment)
		}
	}
}
