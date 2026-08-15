package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunRepositoryClaimsWaitingRunWithStoredEnvironment(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolID, runnerID := "dispatch-pool-"+suffix, "dispatch-runner-"+suffix
	taskID, runID := "dispatch-task-"+suffix, "dispatch-run-"+suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = pool.Exec(ctx, `DELETE FROM runners WHERE id = $1`, runnerID)
		_, _ = pool.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, poolID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, capabilities) VALUES ($1, $2, $1, 'ENABLED', '{}'::jsonb)`, runnerID, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id, last_heartbeat_at) VALUES ($1, $2, $3, now())`, runnerID+"/boot", runnerID, "boot"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTaskRepository(pool).Create(ctx, TaskDefinition{ID: taskID, Name: taskID, RunnerPoolID: poolID, Command: []string{"echo", "ok"}, Environment: map[string]any{"PORT": 8080}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunRepository(pool).Create(ctx, RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "dispatch-idempotency-" + suffix}); err != nil {
		t.Fatal(err)
	}
	candidate, claimed, err := NewRunRepository(pool).ClaimWaiting(ctx, func(candidate DispatchCandidate) ([]byte, error) {
		return []byte("order"), nil
	})
	if err != nil || !claimed {
		t.Fatalf("claim = %#v, claimed=%t, err=%v", candidate, claimed, err)
	}
	if candidate.RunID != runID || candidate.Environment["PORT"] != "8080" {
		t.Fatalf("candidate = %#v", candidate)
	}
}
