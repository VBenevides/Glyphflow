package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNormalizeDisplayName(t *testing.T) {
	if got := NormalizeDisplayName("john.doe-smith@example.com", ""); got != "John Doe Smith" {
		t.Fatalf("derived display name = %q", got)
	}
	if got := NormalizeDisplayName("john@example.com", " Custom Name "); got != "Custom Name" {
		t.Fatalf("custom display name = %q", got)
	}
}

func TestUserRepositoryRoundTrip(t *testing.T) {
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
	repository := NewUserRepository(pool)
	id := "repo-test-" + time.Now().UTC().Format("20060102150405.000000000")
	email := id + "@example.com"
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id) })
	hash, err := platform.DefaultPasswordHasher([]byte("repository-test-pepper")).Hash("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	roleID := "role-" + id
	if _, err := pool.Exec(ctx, `INSERT INTO roles (id, name) VALUES ($1, $2)`, roleID, roleID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID) })
	want := UserRecord{ID: id, Username: email, Email: email, DisplayName: "Test User", Status: StatusActive, Enabled: true}
	if err := repository.Create(ctx, want, hash); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO role_assignments (user_id, role_id, source_type, source_key) VALUES ($1, $2, 'test', $1)`, id, roleID); err != nil {
		t.Fatal(err)
	}
	got, ok, err := repository.FindByEmail(ctx, email)
	if err != nil || !ok || got != want {
		t.Fatalf("FindByEmail = %#v, %v, %v", got, ok, err)
	}
	page, total, err := repository.ListPage(ctx, StatusActive, email, []string{roleID}, 1, 0)
	if err != nil || total != 1 || len(page) != 1 || page[0].ID != id {
		t.Fatalf("ListPage = %#v, total %d, error %v", page, total, err)
	}
	gotHash, ok, err := repository.PasswordHash(ctx, id)
	if err != nil || !ok || gotHash != hash {
		t.Fatalf("PasswordHash = %q, %v, %v", gotHash, ok, err)
	}
	if err := repository.UpdateDisplayName(ctx, id, "Updated User"); err != nil {
		t.Fatal(err)
	}
	if err := repository.SetEnabled(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	got, ok, err = repository.FindByID(ctx, id)
	if err != nil || !ok || got.DisplayName != "Updated User" || got.Enabled {
		t.Fatalf("updated user = %#v, %v, %v", got, ok, err)
	}
}
