package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuditRepositoryPersistsFiltersAndAppendOnlyRows(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	repository := NewAuditRepository(pool)
	id := "audit-test-" + time.Now().UTC().Format("20060102150405.000000000")
	t.Cleanup(pool.Close)
	if err := repository.Append(ctx, AuditEventRecord{ID: id, ActorName: "actor", Method: "POST", Description: "Create task", Endpoint: "/api/v1/tasks", Target: "task-1", Result: "success", RequestInput: map[string]any{"name": "task"}, CorrelationID: "corr", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	events, total, err := repository.Query(ctx, AuditFilter{Actor: "actor", CorrelationID: "corr", Page: 1, Limit: 10})
	if err != nil || total != 1 || len(events) != 1 || events[0].Method != "POST" {
		t.Fatalf("events = %#v, total=%d, err=%v", events, total, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_events SET result = 'failure' WHERE id = $1`, id); err == nil {
		t.Fatal("append-only audit event was updated")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM audit_events WHERE id = $1`, id); err == nil {
		t.Fatal("append-only audit event was deleted")
	}
}
