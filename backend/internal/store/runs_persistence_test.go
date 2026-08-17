package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunRepositoryPersistsIdempotencyAndStateTransitions(t *testing.T) {
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
	poolID, taskID, runID := "run-pool-"+time.Now().UTC().Format("20060102150405.000000000"), "run-task-"+time.Now().UTC().Format("20060102150405.000000000"), "run-"+time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, runID)
		_, _ = pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = pool.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, poolID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(pool)
	if _, err := tasks.Create(ctx, TaskDefinition{ID: taskID, Name: "Run task", RunnerPoolID: poolID, Command: []string{"echo", "run"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	runs := NewRunRepository(pool)
	created, err := runs.Create(ctx, RunDefinition{ID: runID, TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "idempotency-" + runID})
	if err != nil || created.State != "WAITING" {
		t.Fatalf("created run = %#v, err = %v", created, err)
	}
	if _, err := runs.Create(ctx, RunDefinition{ID: runID + "-duplicate", TaskID: taskID, TriggerType: "MANUAL", IdempotencyKey: "idempotency-" + runID}); err == nil {
		t.Fatal("duplicate idempotency key was accepted")
	}
	updated, changed, err := runs.Transition(ctx, runID, []string{"WAITING"}, "CANCELLED")
	if err != nil || !changed || updated.State != "CANCELLED" {
		t.Fatalf("transition = %#v, changed=%t, err=%v", updated, changed, err)
	}
	if _, changed, err := runs.Transition(ctx, runID, []string{"WAITING"}, "CANCELLED"); err != nil || changed {
		t.Fatalf("stale transition changed state: changed=%t, err=%v", changed, err)
	}
}
