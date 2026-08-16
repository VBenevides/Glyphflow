package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResourceRepositoryFencesTransactionalLeases(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	poolID, runnerID, sessionID, taskID, runID, attemptID, resourceID := "resource-pool-"+suffix, "resource-runner-"+suffix, "resource-session-"+suffix, "resource-task-"+suffix, "resource-run-"+suffix, "resource-attempt-"+suffix, "resource-"+suffix
	t.Cleanup(func() {
		for _, cleanup := range []struct {
			name  string
			query string
			id    string
		}{
			{"run", `DELETE FROM runs WHERE id = $1`, runID},
			{"resource", `DELETE FROM resources WHERE id = $1`, resourceID},
		} {
			if _, err := pool.Exec(ctx, cleanup.query, cleanup.id); err != nil {
				t.Errorf("cleanup %s: %v", cleanup.name, err)
			}
		}
		// Task versions are immutable history. This repository test runs in a
		// fresh database in the release gate, so retain its task and pool rows.
		pool.Close()
	})
	if _, err := pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO runners (id, pool_id, name) VALUES ($1, $2, $1)`, runnerID, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO runner_sessions (id, runner_id, boot_id) VALUES ($1, $2, 'boot-1')`, sessionID, runnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTaskRepository(pool).Create(ctx, TaskDefinition{ID: taskID, Name: "Resource task", RunnerPoolID: poolID, Command: []string{"echo", "resource"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunRepository(pool).Create(ctx, RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL"}); err != nil {
		t.Fatal(err)
	}
	if err := NewRunRepository(pool).CreateAttempt(ctx, ExecutionAttemptDefinition{ID: attemptID, RunID: runID, RunnerID: runnerID, RunnerSessionID: sessionID, AttemptNumber: 1, LeaseToken: "attempt-lease-" + suffix, FencingToken: 1, LeaseNotAfter: time.Now().Add(time.Minute), ExecutionSpecDigest: "digest"}); err != nil {
		t.Fatal(err)
	}
	run, found, err := NewRunRepository(pool).Find(ctx, runID)
	if err != nil || !found || run.Runner != runnerID {
		t.Fatalf("run projection = %#v, found=%t, err=%v", run, found, err)
	}
	task, found, err := NewTaskRepository(pool).Find(ctx, taskID)
	if err != nil || !found || task.LatestRun == nil || task.LatestRun.ID != runID || task.LatestRun.TriggerType != "MANUAL" {
		t.Fatalf("task latest run projection = %#v, found=%t, err=%v", task.LatestRun, found, err)
	}
	resources := NewResourceRepository(pool)
	if err := resources.Create(ctx, resourceID, "Resource", "exclusive"); err != nil {
		t.Fatal(err)
	}
	first, err := resources.Acquire(ctx, resourceID, attemptID, time.Minute, time.Now().UTC())
	if err != nil || first.FencingToken != 1 {
		t.Fatalf("first lease = %#v, err = %v", first, err)
	}
	if _, err := resources.Acquire(ctx, resourceID, attemptID, time.Minute, time.Now().UTC()); err == nil {
		t.Fatal("concurrent resource acquisition succeeded")
	}
	if err := resources.Release(ctx, resourceID, attemptID, first.FencingToken); err != nil {
		t.Fatal(err)
	}
	second, err := resources.Acquire(ctx, resourceID, attemptID, time.Minute, time.Now().UTC())
	if err != nil || second.FencingToken != 2 {
		t.Fatalf("second lease = %#v, err = %v", second, err)
	}
	if err := resources.Release(ctx, resourceID, attemptID, first.FencingToken); err == nil {
		t.Fatal("stale fencing token released the lease")
	}
}
