package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ResourceLeaseInput struct {
	ID          string
	ResourceKey string
	LeaseToken  string
	ExpiresAt   time.Time
}

type CreateTaskRunParams struct {
	RunID            string
	TaskDefinitionID string
	OccurrenceAt     time.Time
	RunnerID         string
	Attempt          int
	LeaseToken       string
	OrderBytes       []byte
	OrderSubject     string
	Resources        []ResourceLeaseInput
}

const insertTaskRunSQL = `
INSERT INTO task_runs (id, task_definition_id, occurrence_at, runner_id, state, attempt, lease_token)
VALUES ($1, $2, $3, $4, 'queued', $5, $6)
`

const insertResourceLeaseSQL = `
INSERT INTO resource_leases (id, resource_key, task_run_id, lease_token, expires_at)
VALUES ($1, $2, $3, $4, $5)
`

const insertDispatchOutboxSQL = `
INSERT INTO dispatch_outbox (id, task_run_id, order_bytes, subject)
VALUES ($1, $2, $3, $4)
`

// CreateTaskRun commits the run, all resource leases, and its dispatch outbox
// row together. Any failed insert rolls the whole transaction back.
func CreateTaskRun(ctx context.Context, pool *pgxpool.Pool, params CreateTaskRunParams) error {
	if len(params.OrderBytes) == 0 {
		return fmt.Errorf("order bytes are required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, insertTaskRunSQL, params.RunID, params.TaskDefinitionID, params.OccurrenceAt, params.RunnerID, params.Attempt, params.LeaseToken); err != nil {
		return fmt.Errorf("insert task run: %w", err)
	}
	for _, lease := range params.Resources {
		if _, err := tx.Exec(ctx, insertResourceLeaseSQL, lease.ID, lease.ResourceKey, params.RunID, lease.LeaseToken, lease.ExpiresAt); err != nil {
			return fmt.Errorf("insert resource lease: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, insertDispatchOutboxSQL, params.RunID, params.RunID, params.OrderBytes, params.OrderSubject); err != nil {
		return fmt.Errorf("insert dispatch outbox: %w", err)
	}
	return tx.Commit(ctx)
}
