package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func canonicalMigrationSQL(t *testing.T) string {
	t.Helper()
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "canonical" {
		t.Fatalf("migrations = %#v, want only 001_canonical.sql", migrations)
	}
	return strings.ToLower(migrations[0].SQL)
}

func TestCanonicalSchemaIsSingleCleanBaseline(t *testing.T) {
	directory := filepath.Join("..", "..", "migrations")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var sqlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			sqlFiles = append(sqlFiles, entry.Name())
		}
	}
	if len(sqlFiles) != 1 || sqlFiles[0] != "001_canonical.sql" {
		t.Fatalf("SQL files = %#v, want only 001_canonical.sql", sqlFiles)
	}
	sql := canonicalMigrationSQL(t)
	for _, fragment := range []string{
		"alter table tasks add column",
		"alter table runners add column",
		"drop ",
		"\nupdate ",
		"\ndelete from ",
		"schema_migrations",
	} {
		if strings.Contains(sql, fragment) {
			t.Errorf("canonical schema contains compatibility operation %q", fragment)
		}
	}
}

func TestCanonicalSchemaContainsFinalTables(t *testing.T) {
	sql := canonicalMigrationSQL(t)
	for _, table := range []string{
		"config", "users", "user_passwords", "auth_sessions", "roles", "permissions", "role_permissions", "role_assignments",
		"sso_providers", "encrypted_secrets", "sso_group_role_mappings", "user_sso_identities", "sso_authorization_states", "audit_events", "exit_code",
		"global_variables", "global_variable_references", "runner_pools", "runners", "runner_sessions", "runner_metrics", "runner_keys", "runner_enrollments",
		"tasks", "task_versions", "schedules", "schedule_versions", "runs", "execution_attempts", "run_events", "execution_log_chunks",
		"resources", "task_resource_requirements", "resource_leases", "dispatch_outbox", "event_inbox", "dead_letters", "retention_legal_holds",
	} {
		if !strings.Contains(sql, "create table "+table+" ") {
			t.Errorf("canonical schema does not define %s", table)
		}
	}
}

func TestCanonicalSchemaContainsFinalShape(t *testing.T) {
	sql := canonicalMigrationSQL(t)
	for _, fragment := range []string{
		"duration_seconds integer not null check (duration_seconds > 0)",
		"observed_state text not null default 'pending' check (observed_state in ('pending', 'online', 'offline', 'revoked'))",
		"capacity integer not null default 10",
		"current_capacity integer check (current_capacity > 0)",
		"is_archived boolean not null default false",
		"is_deleted boolean not null default false",
		"resolved_global_variables jsonb not null default '{}'::jsonb",
		"max_memory_used_bytes bigint not null default 0",
		"average_memory_used_bytes bigint not null default 0",
		"foreign key (exit_code) references exit_code(code)",
		"retry_delivery_id text not null default ''",
		"cpu_percent double precision not null",
		"name ~ '^[a-z_][a-z0-9_]*$'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("canonical schema does not contain %q", fragment)
		}
	}
	if strings.Contains(sql, "schedule_type") || strings.Contains(sql, "\n    deleted boolean") {
		t.Fatal("canonical schema contains removed compatibility columns")
	}
	if strings.Contains(sql, "secret_hash") {
		t.Fatal("canonical schema contains plaintext secret hashing")
	}
}

func TestCanonicalSchemaHasIntegrityGuards(t *testing.T) {
	sql := canonicalMigrationSQL(t)
	for _, fragment := range []string{
		"check (name !~ '^state\\.')",
		"foreign key (current_version_id, id) references task_versions",
		"unique index runner_pools_name_ci_idx on runner_pools (lower(name)) where not is_deleted",
		"unique index runners_name_ci_idx on runners (lower(name)) where not is_archived and not is_deleted",
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
			t.Errorf("canonical schema does not contain %q", fragment)
		}
	}
}
