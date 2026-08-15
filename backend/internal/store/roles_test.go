package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleRepositoryRoundTrip(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := NewRoleRepository(pool)
	users := NewUserRepository(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID, roleID := "role-test-"+suffix, "role-test-role-"+suffix
	email := userID + "@example.com"
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	})
	if err := users.Create(ctx, UserRecord{ID: userID, Username: email, Email: email, Enabled: true}, ""); err != nil {
		t.Fatal(err)
	}
	roleName := "operator-" + suffix
	if err := repository.Create(ctx, roleID, roleName, "", []string{"tasks.read"}); err != nil {
		t.Fatal(err)
	}
	if err := repository.Assign(ctx, userID, roleID, "manual", "test"); err != nil {
		t.Fatal(err)
	}
	if err := repository.Assign(ctx, userID, roleID, "system-admin", userID); err != nil {
		t.Fatal(err)
	}
	if err := repository.ReplaceSourceAssignments(ctx, roleID, "system-admin", nil); err != nil {
		t.Fatal(err)
	}
	_, assignments, err := repository.UserRoles(ctx, userID)
	if err != nil || len(assignments) != 1 || assignments[0].SourceType != "manual" {
		t.Fatalf("source reconciliation removed explicit assignment: %#v, %v", assignments, err)
	}
	if err := repository.Assign(ctx, userID, roleID, "system-admin", userID); err != nil {
		t.Fatal(err)
	}
	listed, err := repository.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, listedRole := range listed {
		if listedRole.ID == roleID && listedRole.AssignedUsers != 1 {
			t.Fatalf("assigned users = %d, want 1", listedRole.AssignedUsers)
		}
	}
	if err := repository.Unassign(ctx, userID, roleID); err != nil {
		t.Fatal(err)
	}
	_, assignments, err = repository.UserRoles(ctx, userID)
	if err != nil || len(assignments) != 1 || assignments[0].SourceType != "system-admin" {
		t.Fatalf("manual unassignment removed derived assignment: %#v, %v", assignments, err)
	}
	permissions, err := repository.EffectivePermissions(ctx, userID)
	if err != nil || len(permissions) != 1 || permissions[0] != "tasks.read" {
		t.Fatalf("effective permissions = %#v, %v", permissions, err)
	}
	if err := repository.ReplacePermissions(ctx, roleID, []string{"runs.read"}); err != nil {
		t.Fatal(err)
	}
	permissions, err = repository.EffectivePermissions(ctx, userID)
	if err != nil || len(permissions) != 1 || permissions[0] != "runs.read" {
		t.Fatalf("updated permissions = %#v, %v", permissions, err)
	}
	renamed := "renamed-" + suffix
	if err := repository.Rename(ctx, roleID, renamed); err != nil {
		t.Fatal(err)
	}
	role, ok, err := repository.FindByID(ctx, roleID)
	if err != nil || !ok || role.Name != renamed {
		t.Fatalf("renamed role = %#v, %v, %v", role, ok, err)
	}
	roles, assignments, err := repository.UserRoles(ctx, userID)
	if err != nil || len(roles) != 1 || len(assignments) != 1 || roles[0].ID != roleID {
		t.Fatalf("user roles = %#v, assignments = %#v, err = %v", roles, assignments, err)
	}
}
