package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSessionRepositoryRotationAndReplayRevocation(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL session tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	userID := "session-test-" + suffix
	email := userID + "@example.com"
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, username, email) VALUES ($1, $2, $3)`, userID, email, email); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	repository := NewSessionRepository(pool)
	now := time.Now().UTC()
	oldToken := "old-refresh-" + suffix
	first := SessionRecord{ID: "session-" + suffix, UserID: userID, RefreshTokenHash: platform.HashToken(oldToken), AccessExpiresAt: now.Add(time.Minute), RefreshExpiresAt: now.Add(time.Hour), SessionFamilyID: "family-" + suffix, LastSeenAt: now}
	if err := repository.Create(ctx, first); err != nil {
		t.Fatal(err)
	}
	replacement := SessionRecord{ID: "session-next-" + suffix, RefreshTokenHash: platform.HashToken("next-refresh-" + suffix), AccessExpiresAt: now.Add(2 * time.Minute), RefreshExpiresAt: now.Add(2 * time.Hour), LastSeenAt: now}
	if err := repository.Rotate(ctx, first.ID, first.RefreshTokenHash, replacement); err != nil {
		t.Fatal(err)
	}
	if err := repository.Rotate(ctx, first.ID, first.RefreshTokenHash, replacement); err != ErrSessionReplay {
		t.Fatalf("replay error = %v, want ErrSessionReplay", err)
	}
	active, err := repository.Active(ctx, replacement.ID, userID)
	if err != nil || active {
		t.Fatalf("replacement active after family replay = %v, %v", active, err)
	}
	old := SessionRecord{ID: "session-old-" + suffix, UserID: userID, RefreshTokenHash: platform.HashToken("old-cleanup-" + suffix), AccessExpiresAt: now.Add(time.Minute), RefreshExpiresAt: now.Add(time.Hour), SessionFamilyID: "family-old-" + suffix, LastSeenAt: now}
	if err := repository.Create(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE auth_sessions SET created_at = $2 WHERE id = $1`, old.ID, now.Add(-15*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteOlderThan(ctx, now.Add(-14*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, found, err := repository.Get(ctx, old.ID); err != nil || found {
		t.Fatalf("old session found after cleanup = %v, %v", found, err)
	}
}
