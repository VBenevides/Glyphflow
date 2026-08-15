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

func TestMigrationsHaveCanonicalBaseline(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 1 || migrations[0].Version != 1 || migrations[0].Name != "canonical" {
		t.Fatalf("migrations = %#v, want 001_canonical.sql as the baseline", migrations)
	}
	for _, migration := range migrations {
		if strings.Contains(migration.SQL, "_v1") || strings.Contains(migration.SQL, "_v2") || strings.Contains(migration.SQL, "_vx") {
			t.Fatalf("migration %d contains a version-suffixed identifier", migration.Version)
		}
	}
}

func TestRunnerPendingStateMigration(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) < 2 || migrations[1].Version != 2 || migrations[1].Name != "runner_pending_state" {
		t.Fatalf("migrations = %#v, want runner pending state migration", migrations)
	}
	sql := strings.ToLower(migrations[1].SQL)
	for _, fragment := range []string{"drop constraint", "runners_observed_state_check", "'pending'", "set default 'pending'"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("pending state migration does not contain %q", fragment)
		}
	}
}

func TestRunnerCapacityDefaultMigration(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var capacityMigration Migration
	for _, migration := range migrations {
		if migration.Name == "runner_capacity_default" {
			capacityMigration = migration
		}
	}
	if capacityMigration.Version != 10 || !strings.Contains(strings.ToLower(capacityMigration.SQL), "alter table runners alter column capacity set default 10") {
		t.Fatalf("runner capacity migration = %#v", capacityMigration)
	}
}

func TestRunnerCurrentCapacityMigration(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var currentCapacity Migration
	for _, migration := range migrations {
		if migration.Name == "runner_current_capacity" {
			currentCapacity = migration
		}
	}
	if currentCapacity.Version != 11 || !strings.Contains(strings.ToLower(currentCapacity.SQL), "add column current_capacity") {
		t.Fatalf("runner current capacity migration = %#v", currentCapacity)
	}
}

func TestGlobalVariableMigration(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var globalMigration Migration
	for _, migration := range migrations {
		if migration.Name == "global_variables" {
			globalMigration = migration
		}
	}
	if globalMigration.Version != 6 {
		t.Fatalf("migrations = %#v, want global variable migration", migrations)
	}
	sql := strings.ToLower(globalMigration.SQL)
	for _, fragment := range []string{"create table global_variables", "global_variable_references", "on delete restrict"} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("global variable migration does not contain %q", fragment)
		}
	}
}

func TestCronOnlyMigration(t *testing.T) {
	migrations, err := LoadMigrations(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	var cronMigration Migration
	for _, migration := range migrations {
		if migration.Name == "cron_only" {
			cronMigration = migration
		}
	}
	if cronMigration.Version != 7 || !strings.Contains(strings.ToLower(cronMigration.SQL), "drop column schedule_type") {
		t.Fatalf("cron-only migration = %#v", cronMigration)
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
		"check (name !~ '^state\\.')",
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
