package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTaskRepositoryKeepsImmutableVersionsAndPointer(t *testing.T) {
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
	repository := NewTaskRepository(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	poolID, taskID := "task-pool-"+suffix, "task-"+suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, taskID)
		_, _ = pool.Exec(ctx, `DELETE FROM runner_pools WHERE id = $1`, poolID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(ctx, TaskDefinition{ID: taskID, Name: "Nightly", RunnerPoolID: poolID, Command: []string{"echo", "one"}, Enabled: true})
	if err != nil || created.ActiveVersion != 1 || created.Command[1] != "one" {
		t.Fatalf("created task = %#v, err = %v", created, err)
	}
	updated, err := repository.CreateVersion(ctx, taskID, TaskDefinition{Name: "Nightly v2", Command: []string{"echo", "two"}})
	if err != nil || updated.ActiveVersion != 2 || updated.Command[1] != "two" {
		t.Fatalf("updated task = %#v, err = %v", updated, err)
	}
	versions, err := repository.ListVersions(ctx, taskID)
	if err != nil || len(versions) != 2 || versions[0].Version != 2 || versions[1].Version != 1 {
		t.Fatalf("task versions = %#v, err = %v", versions, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_versions SET working_directory = '/tmp' WHERE task_id = $1`, taskID); err == nil {
		t.Fatal("immutable task version was updated")
	}
	if _, err := repository.CreateVersion(ctx, taskID, TaskDefinition{Command: []string{"echo", "bad"}, RunnerPoolID: "missing-pool"}); err == nil {
		t.Fatal("invalid task version was accepted")
	}
	current, found, err := repository.Find(ctx, taskID)
	if err != nil || !found || current.ActiveVersion != 2 {
		t.Fatalf("failed version moved pointer: %#v, found=%t, err=%v", current, found, err)
	}
	deleted, err := repository.Delete(ctx, taskID)
	if err != nil || !deleted {
		t.Fatalf("task deletion failed: deleted=%t, err=%v", deleted, err)
	}
	var isDeleted bool
	if err := pool.QueryRow(ctx, `SELECT is_deleted FROM tasks WHERE id = $1`, taskID).Scan(&isDeleted); err != nil || !isDeleted {
		t.Fatalf("task was not soft deleted: deleted=%t, err=%v", isDeleted, err)
	}
	if _, found, err := repository.Find(ctx, taskID); err != nil || found {
		t.Fatalf("deleted task still exists: found=%t, err=%v", found, err)
	}
}
