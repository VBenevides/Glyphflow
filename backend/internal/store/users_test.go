package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

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
	want := UserRecord{ID: id, Username: email, Email: email, DisplayName: "Test User", Enabled: true}
	if err := repository.Create(ctx, want, hash); err != nil {
		t.Fatal(err)
	}
	got, ok, err := repository.FindByEmail(ctx, email)
	if err != nil || !ok || got != want {
		t.Fatalf("FindByEmail = %#v, %v, %v", got, ok, err)
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
