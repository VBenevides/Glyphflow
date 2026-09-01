package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScheduleRepositoryKeepsImmutableVersionsAndPointer(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	tasks := NewTaskRepository(pool)
	schedules := NewScheduleRepository(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	poolID, taskID, scheduleID := "schedule-pool-"+suffix, "schedule-task-"+suffix, "schedule-"+suffix
	var scheduledRunID string
	t.Cleanup(func() {
		if scheduledRunID != "" {
			if _, err := pool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, scheduledRunID); err != nil {
				t.Errorf("cleanup scheduled run: %v", err)
			}
		}
		if _, err := pool.Exec(ctx, `DELETE FROM schedules WHERE id = $1`, scheduleID); err != nil {
			t.Errorf("cleanup schedule: %v", err)
		}
		// Task versions are immutable history. This repository test runs in a
		// fresh database in the release gate, so retain its task and pool rows.
		pool.Close()
	})
	if _, err := pool.Exec(ctx, `INSERT INTO runner_pools (id, name) VALUES ($1, $1)`, poolID); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Create(ctx, TaskDefinition{ID: taskID, Name: "Scheduled task", RunnerPoolID: poolID, Command: []string{"echo", "one"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	created, err := schedules.Create(ctx, ScheduleDefinition{ID: scheduleID, Name: "Hourly", TaskID: taskID, Expression: "0 * * * *", Timezone: "UTC", Enabled: true})
	if err != nil || created.ActiveVersion != 1 {
		t.Fatalf("created schedule = %#v, err = %v", created, err)
	}
	updated, err := schedules.Update(ctx, scheduleID, ScheduleDefinition{Name: "Every hour", TaskID: taskID, Expression: "30 * * * *", Timezone: "UTC", Enabled: true})
	if err != nil || updated.ActiveVersion != 2 || updated.Expression != "30 * * * *" {
		t.Fatalf("updated schedule = %#v, err = %v", updated, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE schedule_versions SET expression = '*/5 * * * *' WHERE schedule_id = $1`, scheduleID); err == nil {
		t.Fatal("immutable schedule version was updated")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM schedule_versions WHERE id = $1`, scheduleID+"-v1"); err == nil {
		t.Fatal("immutable schedule version was deleted")
	}
	if _, err := schedules.Update(ctx, scheduleID, ScheduleDefinition{Name: "Broken", TaskID: "missing-task", Expression: "0 * * * *", Timezone: "UTC", Enabled: true}); err == nil {
		t.Fatal("schedule with missing task was accepted")
	}
	current, found, err := schedules.Find(ctx, scheduleID)
	if err != nil || !found || current.ActiveVersion != 2 {
		t.Fatalf("failed schedule update moved pointer: %#v, found=%t, err=%v", current, found, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE schedules SET next_fire_at = now() WHERE id = $1`, scheduleID); err != nil {
		t.Fatal(err)
	}
	scheduledRunID, changed, err := schedules.CreateDueRun(ctx, time.Now().UTC(), func(due DueScheduleRecord) (time.Time, error) {
		return due.NextFireAt.Add(time.Minute), nil
	})
	if err != nil || !changed || scheduledRunID == "" {
		t.Fatalf("scheduled run = %q, changed=%t, err=%v", scheduledRunID, changed, err)
	}
	if next, found, err := schedules.Find(ctx, scheduleID); err != nil || !found || next.NextFireAt == nil {
		t.Fatalf("schedule next fire = %#v, found=%t, err=%v", next.NextFireAt, found, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM runs WHERE id = $1`, scheduledRunID); err != nil {
		t.Fatal(err)
	}
	scheduledRunID = ""
	if _, err := schedules.Create(ctx, ScheduleDefinition{ID: scheduleID + "-bad", Name: "Bad timezone", TaskID: taskID, Expression: "0 * * * *", Timezone: "Not/AZone", Enabled: true}); err == nil {
		t.Fatal("invalid timezone was accepted")
	}
	deleted, err := schedules.Delete(ctx, scheduleID)
	if err != nil || !deleted {
		t.Fatalf("schedule deletion failed: deleted=%t, err=%v", deleted, err)
	}
	if _, found, err := schedules.Find(ctx, scheduleID); err != nil || found {
		t.Fatalf("deleted schedule still exists: found=%t, err=%v", found, err)
	}
}

func TestScheduleDeadlineDefaultsAndMinimum(t *testing.T) {
	definition := ScheduleDefinition{ID: "schedule-1", Name: "Hourly", TaskID: "task-1", Expression: "0 * * * *", Timezone: "UTC"}
	if got := normalizeScheduleDefinition(definition).DeadlineSeconds; got != defaultStartDeadlineSeconds {
		t.Fatalf("default deadline = %d, want %d", got, defaultStartDeadlineSeconds)
	}
	definition.DeadlineSeconds = minimumStartDeadlineSeconds - 1
	if err := validateScheduleDefinition(definition); err == nil {
		t.Fatal("deadline below minimum was accepted")
	}
}
