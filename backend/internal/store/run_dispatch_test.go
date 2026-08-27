package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunRepositoryClaimsAndReconcilesStartFailure(t *testing.T) {
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
	if _, err := NewTaskRepository(pool).Create(ctx, TaskDefinition{ID: taskID, Name: taskID, RunnerPoolID: poolID, Command: []string{"echo", "ok"}, Environment: map[string]any{"PORT": 8080}, DurationSeconds: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunRepository(pool).Create(ctx, RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL", ScheduledFor: time.Now().UTC().Add(-11 * time.Minute), IdempotencyKey: "dispatch-idempotency-" + suffix}); err != nil {
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
	if _, err := pool.Exec(ctx, `UPDATE execution_attempts SET dispatched_at = $2 WHERE run_id = $1`, runID, time.Now().UTC().Add(-11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := NewRunRepository(pool).ReconcileTimedOutDispatches(ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var runState, attemptState, outboxState string
	if err := pool.QueryRow(ctx, `SELECT r.state, a.state, o.state FROM runs r JOIN execution_attempts a ON a.run_id = r.id JOIN dispatch_outbox o ON o.execution_attempt_id = a.id WHERE r.id = $1`, runID).Scan(&runState, &attemptState, &outboxState); err != nil {
		t.Fatal(err)
	}
	if runState != "FAILED" || attemptState != "FAILED" || outboxState != "FAILED" {
		t.Fatalf("states = run %q, attempt %q, outbox %q", runState, attemptState, outboxState)
	}
	var exitCode int
	if err := pool.QueryRow(ctx, `SELECT exit_code FROM execution_attempts WHERE run_id = $1`, runID).Scan(&exitCode); err != nil {
		t.Fatal(err)
	}
	if exitCode != 5 {
		t.Fatalf("start failure exit code = %d, want 5", exitCode)
	}
	_, granted, err := NewRunRepository(pool).ClaimStart(ctx, StartClaimInput{RunID: candidate.RunID, RunnerID: candidate.RunnerID, RunnerSessionID: candidate.RunnerSessionID, LeaseToken: candidate.LeaseToken, Attempt: candidate.AttemptNumber, FencingToken: candidate.FencingToken, ExecutionSpecDigest: candidate.ExecutionSpecDigest})
	if err != nil {
		t.Fatal(err)
	}
	if granted {
		t.Fatal("start claim succeeded after Start Failure")
	}
}

func TestRunRepositoryClaimsStartBeforeTimeout(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolID, runnerID := "start-pool-"+suffix, "start-runner-"+suffix
	taskID, runID := "start-task-"+suffix, "start-run-"+suffix
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `DELETE FROM runs WHERE id = $1`, runID)
		_, _ = db.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = db.Exec(ctx, `DELETE FROM runners WHERE id = $1`, runnerID)
		_, _ = db.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, poolID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, capabilities) VALUES ($1, $2, $1, 'ENABLED', '{}'::jsonb)`, runnerID, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id, last_heartbeat_at) VALUES ($1, $2, $3, now())`, runnerID+"/boot", runnerID, "boot"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTaskRepository(db).Create(ctx, TaskDefinition{ID: taskID, Name: taskID, RunnerPoolID: poolID, Command: []string{"echo", "ok"}, DurationSeconds: 1, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunRepository(db).Create(ctx, RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "start-idempotency-" + suffix}); err != nil {
		t.Fatal(err)
	}
	candidate, claimed, err := NewRunRepository(db).ClaimWaiting(ctx, func(candidate DispatchCandidate) ([]byte, error) { return []byte("order"), nil })
	if err != nil || !claimed {
		t.Fatalf("claim = %#v, claimed=%t, err=%v", candidate, claimed, err)
	}
	runs := NewRunRepository(db)
	grantedAt, granted, err := runs.ClaimStart(ctx, StartClaimInput{RunID: candidate.RunID, RunnerID: candidate.RunnerID, RunnerSessionID: candidate.RunnerSessionID, LeaseToken: candidate.LeaseToken, Attempt: candidate.AttemptNumber, FencingToken: candidate.FencingToken, ExecutionSpecDigest: candidate.ExecutionSpecDigest})
	if err != nil || !granted || grantedAt.IsZero() {
		t.Fatalf("start claim = %v, granted=%t, err=%v", grantedAt, granted, err)
	}
	var runState, attemptState string
	if err := db.QueryRow(ctx, `SELECT r.state, a.state FROM runs r JOIN execution_attempts a ON a.run_id = r.id WHERE r.id = $1`, runID).Scan(&runState, &attemptState); err != nil {
		t.Fatal(err)
	}
	if runState != "RUNNING" || attemptState != "RUNNING" {
		t.Fatalf("states = run %q, attempt %q", runState, attemptState)
	}
	if _, granted, err := runs.ClaimStart(ctx, StartClaimInput{RunID: candidate.RunID, RunnerID: candidate.RunnerID, RunnerSessionID: candidate.RunnerSessionID, LeaseToken: candidate.LeaseToken, Attempt: candidate.AttemptNumber, FencingToken: candidate.FencingToken, ExecutionSpecDigest: candidate.ExecutionSpecDigest}); err != nil || !granted {
		t.Fatalf("idempotent start claim = granted=%t, err=%v", granted, err)
	}
}

func TestRunRepositoryReconcilesStaleCancellation(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolID, runnerID := "cancel-pool-"+suffix, "cancel-runner-"+suffix
	taskID, runID := "cancel-task-"+suffix, "cancel-run-"+suffix
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `DELETE FROM runs WHERE id = $1`, runID)
		_, _ = db.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = db.Exec(ctx, `DELETE FROM runners WHERE id = $1`, runnerID)
		_, _ = db.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, poolID)
	})
	if _, err := db.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, capabilities) VALUES ($1, $2, $1, 'ENABLED', '{}'::jsonb)`, runnerID, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id, last_heartbeat_at) VALUES ($1, $2, $3, now())`, runnerID+"/boot", runnerID, "boot"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTaskRepository(db).Create(ctx, TaskDefinition{ID: taskID, Name: taskID, RunnerPoolID: poolID, Command: []string{"echo", "ok"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunRepository(db).Create(ctx, RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "cancel-idempotency-" + suffix}); err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := NewRunRepository(db).ClaimWaiting(ctx, func(DispatchCandidate) ([]byte, error) { return []byte("order"), nil }); err != nil || !claimed {
		t.Fatalf("claim cancellation candidate setup: claimed=%t err=%v", claimed, err)
	}
	runs := NewRunRepository(db)
	if _, changed, err := runs.RequestCancellation(ctx, runID, "stop"); err != nil || !changed {
		t.Fatalf("first cancellation: changed=%t err=%v", changed, err)
	}
	if _, changed, err := runs.RequestCancellation(ctx, runID, "stop again"); err != nil || changed {
		t.Fatalf("duplicate cancellation changed state: changed=%t err=%v", changed, err)
	}
	if _, err := db.Exec(ctx, `UPDATE runs SET state = 'CANCELLING', updated_at = now() - interval '1 minute' WHERE id = $1`, runID); err != nil {
		t.Fatal(err)
	}
	if err := NewRunRepository(db).ReconcileStaleCancellations(ctx, time.Now().UTC().Add(-30*time.Second)); err != nil {
		t.Fatal(err)
	}
	var runState, attemptState string
	if err := db.QueryRow(ctx, `SELECT r.state, a.state FROM runs r JOIN execution_attempts a ON a.run_id = r.id WHERE r.id = $1`, runID).Scan(&runState, &attemptState); err != nil {
		t.Fatal(err)
	}
	if runState != "UNKNOWN" || attemptState != "UNKNOWN" {
		t.Fatalf("states = run %q, attempt %q", runState, attemptState)
	}
	var activeCount int
	if err := db.QueryRow(ctx, `SELECT active_count FROM runners WHERE id = $1`, runnerID).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if activeCount != 0 {
		t.Fatalf("active count = %d, want 0", activeCount)
	}
	var eventKind string
	if err := db.QueryRow(ctx, `SELECT event_kind FROM run_events WHERE execution_attempt_id = (SELECT id FROM execution_attempts WHERE run_id = $1)`, runID).Scan(&eventKind); err != nil {
		t.Fatal(err)
	}
	if eventKind != "unknown" {
		t.Fatalf("event kind = %q", eventKind)
	}
}

func TestRunRepositoryReportsWaitingPlacementBlocker(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	poolID, runnerID := "blocker-pool-"+suffix, "blocker-runner-"+suffix
	taskID, runID := "blocker-task-"+suffix, "blocker-run-"+suffix
	t.Cleanup(func() {
		if _, err := db.Exec(ctx, `DELETE FROM runs WHERE id = $1`, runID); err != nil {
			t.Errorf("cleanup run: %v", err)
		}
		if _, err := NewTaskRepository(db).Delete(ctx, taskID); err != nil {
			t.Errorf("cleanup task: %v", err)
		}
		if _, err := db.Exec(ctx, `DELETE FROM runners WHERE id = $1`, runnerID); err != nil {
			t.Errorf("cleanup runner: %v", err)
		}
		if _, err := db.Exec(ctx, `UPDATE runner_pools SET enabled = false, is_deleted = true WHERE id = $1`, poolID); err != nil {
			t.Errorf("cleanup pool: %v", err)
		}
	})
	if _, err := db.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO runners (id, pool_id, name, desired_state, capabilities) VALUES ($1, $2, $1, 'ENABLED', '{}'::jsonb)`, runnerID, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTaskRepository(db).Create(ctx, TaskDefinition{ID: taskID, Name: taskID, RunnerPoolID: poolID, Command: []string{"echo", "ok"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunRepository(db).Create(ctx, RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "blocker-idempotency-" + suffix}); err != nil {
		t.Fatal(err)
	}
	run, found, err := NewRunRepository(db).Find(ctx, runID)
	if err != nil || !found {
		t.Fatalf("find waiting run = %#v, found=%t, err=%v", run, found, err)
	}
	if run.PlacementBlocker != "All matching runners are offline." {
		t.Fatalf("placement blocker = %q", run.PlacementBlocker)
	}
}
