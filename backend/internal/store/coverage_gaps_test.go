package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSQLiteDatabaseAdaptersAndMigrationErrors(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)
	adapter := sqliteDatabase{db: db}

	if _, err := databaseFrom(adapter); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenSQLite(" "); err == nil {
		t.Fatal("empty SQLite path was accepted")
	}
	if memory, err := OpenSQLite(":memory:"); err != nil {
		t.Fatal(err)
	} else {
		memory.Close()
	}

	if _, err := adapter.Exec(ctx, "CREATE TABLE adapter_values (value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Exec(ctx, "INSERT INTO adapter_values (value) VALUES ($1)", "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Exec(ctx, "INSERT INTO missing_adapter (value) VALUES ($1)", "one"); err == nil {
		t.Fatal("invalid adapter exec succeeded")
	}

	rows, err := adapter.Query(ctx, "SELECT value FROM adapter_values")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("adapter query returned no rows")
	}
	var value string
	if err := rows.Scan(&value); err != nil || value != "one" {
		t.Fatalf("adapter query scan = %q, %v", value, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Query(ctx, "SELECT value FROM missing_adapter"); err == nil {
		t.Fatal("invalid adapter query succeeded")
	}

	rows, err = adapter.Query(ctx, "SELECT value FROM adapter_values")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("adapter scan error query returned no rows")
	}
	if err := rows.Scan(&value, &value); err == nil {
		t.Fatal("invalid adapter scan succeeded")
	}
	rows.Close()

	if err := adapter.QueryRow(ctx, "SELECT value FROM adapter_values").Scan(&value); err != nil || value != "one" {
		t.Fatalf("adapter query row = %q, %v", value, err)
	}
	if err := adapter.QueryRow(ctx, "SELECT value FROM adapter_values WHERE false").Scan(&value); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("empty adapter query row = %v", err)
	}
	if err := adapter.QueryRow(ctx, "SELECT value FROM missing_adapter").Scan(&value); err == nil {
		t.Fatal("invalid adapter query row succeeded")
	}

	tx, err := adapter.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO adapter_values (value) VALUES ($1)", "two"); err != nil {
		t.Fatal(err)
	}
	txRows, err := tx.Query(ctx, "SELECT value FROM adapter_values ORDER BY value")
	if err != nil {
		t.Fatal(err)
	}
	if !txRows.Next() {
		t.Fatal("transaction query returned no rows")
	}
	txRows.Close()
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM adapter_values").Scan(new(int)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = adapter.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}

	if err := ApplySQLiteMigrations(ctx, db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE schema_migrations SET checksum = 'changed'"); err != nil {
		t.Fatal(err)
	}
	if err := ApplySQLiteMigrations(ctx, db, "../../migrations"); err == nil {
		t.Fatal("changed migration checksum was accepted")
	}
}

func TestSQLiteMigrationLoaderErrors(t *testing.T) {
	if _, err := LoadMigrations(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing migration directory was accepted")
	}
	for name := range map[string]struct{}{"bad.sql": {}, "0_zero.sql": {}, "one.sql": {}} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("SELECT 1"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadMigrations(dir); err == nil {
			t.Fatalf("invalid migration %q was accepted", name)
		}
	}
	if err := ApplySQLiteMigrations(context.Background(), coverageSQLite(t), filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("invalid SQLite migration directory was accepted")
	}
}

func TestSQLiteConfigGlobalVariableAndExitCodeEdges(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)

	config := NewConfigStore(db)
	if _, err := config.Get(ctx, "NOT_ALLOWED", new(string)); err == nil {
		t.Fatal("invalid config read succeeded")
	}
	if err := config.SetIfAbsent(ctx, "WEB_ORIGIN", "http://localhost"); err != nil {
		t.Fatal(err)
	}
	if err := config.SetIfAbsent(ctx, "WEB_ORIGIN", "http://ignored"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO config (name, value) VALUES (?, ?)", "MAX_MESSAGE_BYTES", "not-json"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Get(ctx, "MAX_MESSAGE_BYTES", new(int)); err == nil {
		t.Fatal("invalid config JSON was accepted")
	}
	if err := config.SetAuthenticationSettings(ctx, true, true, "missing-role"); err == nil {
		t.Fatal("missing default role was accepted")
	}

	variables := NewGlobalVariableRepository(db)
	if _, found, err := variables.Find(ctx, "missing"); err != nil || found {
		t.Fatalf("missing variable = found %v err %v", found, err)
	}
	if _, err := variables.Update(ctx, "missing", "MISSING", "value"); err == nil {
		t.Fatal("missing variable update succeeded")
	}
	if err := variables.Delete(ctx, "missing"); err == nil {
		t.Fatal("missing variable delete succeeded")
	}
	if _, err := variables.Create(ctx, "variable-edge", "EDGE", "value"); err != nil {
		t.Fatal(err)
	}
	if _, err := variables.Create(ctx, "variable-edge-duplicate", "EDGE", "value"); err == nil {
		t.Fatal("duplicate variable name succeeded")
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO global_variable_references (variable_id, owner_type, owner_id) VALUES (?, ?, ?)", "variable-edge", "task_version", "owner"); err != nil {
		t.Fatal(err)
	}
	if _, err := variables.Update(ctx, "variable-edge", "OTHER", "value"); err == nil {
		t.Fatal("referenced variable rename succeeded")
	}
	if err := variables.Delete(ctx, "variable-edge"); err != nil {
		t.Fatal(err)
	}

	exitCodes := NewExitCodeRepository(db)
	if _, err := exitCodes.Create(ctx, 40, " "); err == nil {
		t.Fatal("blank exit code meaning was accepted")
	}
	if _, err := exitCodes.Create(ctx, 0, "duplicate"); err == nil {
		t.Fatal("duplicate system exit code was accepted")
	}
	if _, err := exitCodes.Update(ctx, 999, 1000, "missing"); !errors.Is(err, ErrExitCodeNotFound) {
		t.Fatalf("missing exit code update = %v", err)
	}
	if _, err := exitCodes.Update(ctx, 0, 1000, "system"); !errors.Is(err, ErrExitCodeSystem) {
		t.Fatalf("system exit code update = %v", err)
	}
	if err := exitCodes.Delete(ctx, 999); !errors.Is(err, ErrExitCodeNotFound) {
		t.Fatalf("missing exit code delete = %v", err)
	}
	if err := exitCodes.Delete(ctx, 0); !errors.Is(err, ErrExitCodeSystem) {
		t.Fatalf("system exit code delete = %v", err)
	}
	if _, err := exitCodes.Create(ctx, 40, "Custom"); err != nil {
		t.Fatal(err)
	}
	if _, err := exitCodes.Update(ctx, 40, 0, "conflict"); err == nil {
		t.Fatal("exit code conflict was accepted")
	}
	if err := exitCodes.Delete(ctx, 40); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteRoleResourceSessionAndSSOEdges(t *testing.T) {
	ctx := context.Background()
	db := coverageSQLite(t)

	roles := NewRoleRepository(db)
	if err := roles.Ensure(ctx, "role-system-edge", "System edge", "", true, nil); err != nil {
		t.Fatal(err)
	}
	if err := roles.Rename(ctx, "missing", "Missing"); err == nil {
		t.Fatal("missing role rename succeeded")
	}
	if err := roles.Rename(ctx, "role-system-edge", "Changed"); err == nil {
		t.Fatal("system role rename succeeded")
	}
	if err := roles.ReplacePermissions(ctx, "missing", nil); err == nil {
		t.Fatal("missing role permissions update succeeded")
	}
	if err := roles.ReplacePermissions(ctx, "role-system-edge", nil); err == nil {
		t.Fatal("system role permissions update succeeded")
	}
	if err := roles.Delete(ctx, "missing"); err == nil {
		t.Fatal("missing role delete succeeded")
	}
	if err := roles.Delete(ctx, "role-system-edge"); err == nil {
		t.Fatal("system role delete succeeded")
	}
	if err := roles.Create(ctx, "role-edge", "Edge", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := roles.ReplaceSourceAssignments(ctx, "missing", "manual", nil); err == nil {
		t.Fatal("missing role source assignment succeeded")
	}
	if err := roles.Delete(ctx, "role-edge"); err != nil {
		t.Fatal(err)
	}

	resources := NewResourceRepository(db)
	if _, err := resources.Acquire(ctx, "resource-edge", "", time.Minute, time.Now()); err == nil {
		t.Fatal("incomplete resource lease succeeded")
	}

	sessions := NewSessionRepository(db)
	if _, found, err := sessions.Get(ctx, "missing"); err != nil || found {
		t.Fatalf("missing session = found %v err %v", found, err)
	}
	if err := sessions.DeleteOlderThan(ctx, time.Time{}); err == nil {
		t.Fatal("zero session cleanup cutoff was accepted")
	}
	if _, _, err := sessions.ListAdminPage(ctx, "", 0, -1); err != nil {
		t.Fatal(err)
	}

	providers := NewOIDCProviderRepository(db)
	if _, found, err := providers.Find(ctx, "missing"); err != nil || found {
		t.Fatalf("missing provider = found %v err %v", found, err)
	}
	if _, found, err := providers.FindIdentity(ctx, "missing", "subject"); err != nil || found {
		t.Fatalf("missing identity = found %v err %v", found, err)
	}
	if err := providers.DeleteIdentity(ctx, "missing", "missing", "subject"); err == nil {
		t.Fatal("missing identity delete succeeded")
	}
	if err := providers.DeleteGroupRoleMapping(ctx, "missing", "group", "role"); err == nil {
		t.Fatal("missing group mapping delete succeeded")
	}
	if err := providers.ReplaceGroupRoleMappings(ctx, "provider-edge", []SSOGroupRoleMappingRecord{{ProviderID: "provider-edge", GroupName: "", RoleID: "role"}, {ProviderID: "provider-edge", GroupName: "group", RoleID: "role"}}); err != nil {
		t.Fatal(err)
	}
	if err := providers.DeleteGroupRoleMapping(ctx, "provider-edge", "group", "role"); err != nil {
		t.Fatal(err)
	}
}
