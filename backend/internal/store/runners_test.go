package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunnerRepositoryConsumesEnrollmentOnceAndProtectsActiveSessions(t *testing.T) {
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
	repository := NewRunnerRepository(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	runnerID := "runner-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM runners WHERE id = $1`, runnerID)
		_, _ = pool.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, "pool-"+suffix)
	})
	if err := repository.EnsurePool(ctx, "pool-"+suffix, "pool-"+suffix); err != nil {
		t.Fatal(err)
	}
	plain := "enrollment-" + suffix
	digest := sha256.Sum256([]byte(plain))
	if err := repository.CreateEnrollment(ctx, RunnerRecord{ID: runnerID, Name: runnerID, Pool: "pool-" + suffix, Capacity: 1}, RunnerEnrollmentRecord{ID: "enrollment-" + suffix, RunnerID: runnerID, TokenHash: hex.EncodeToString(digest[:]), ExpiresAt: time.Now().Add(time.Minute), Target: runnerID, Artifact: map[string]any{"platform": "linux"}}); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM runner_enrollments WHERE id = $1`, "enrollment-"+suffix).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), plain) {
		t.Fatal("enrollment token was stored in plaintext")
	}
	replacement := "replacement-" + suffix
	replacementDigest := sha256.Sum256([]byte(replacement))
	if err := repository.CreateEnrollment(ctx, RunnerRecord{ID: runnerID, Name: runnerID, Pool: "pool-" + suffix, Capacity: 1}, RunnerEnrollmentRecord{ID: "replacement-enrollment-" + suffix, RunnerID: runnerID, TokenHash: hex.EncodeToString(replacementDigest[:]), ExpiresAt: time.Now().Add(time.Minute), Target: runnerID, Artifact: map[string]any{"platform": "linux"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConsumeEnrollment(ctx, hex.EncodeToString(digest[:]), time.Now()); err == nil {
		t.Fatal("replaced enrollment was accepted")
	}
	if _, err := repository.ConsumeEnrollment(ctx, hex.EncodeToString(replacementDigest[:]), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConsumeEnrollment(ctx, hex.EncodeToString(replacementDigest[:]), time.Now()); err == nil {
		t.Fatal("enrollment replay accepted")
	}
	if err := repository.CreateSession(ctx, runnerID, "boot-1"); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateSession(ctx, runnerID, "boot-2"); err == nil {
		t.Fatal("second active runner session accepted")
	}
}

func TestRunnerCapacityDefaultsToTenWithoutAnUpperCap(t *testing.T) {
	for value, want := range map[int]int{0: 10, -1: 10, 1: 1, 1000: 1000} {
		if got := maxRunnerCapacity(value); got != want {
			t.Fatalf("maxRunnerCapacity(%d) = %d, want %d", value, got, want)
		}
	}
}
