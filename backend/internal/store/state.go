package store

import (
	"context"
	"fmt"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

const updateTaskRunStateSQL = `
UPDATE task_runs
SET state = $1, result = $4, state_version = state_version + 1, updated_at = now()
WHERE id = $2 AND state = $3 AND state_version = $5
`

// UpdateTaskRunState performs an optimistic state transition. A false result
// means another writer changed the run since expectedVersion was read.
func UpdateTaskRunState(ctx context.Context, pool *pgxpool.Pool, runID, fromState, toState string, expectedVersion int64, result []byte) (bool, error) {
	if !platform.TransitionAllowed(fromState, toState) {
		return false, fmt.Errorf("invalid task state transition %q to %q", fromState, toState)
	}
	tag, err := pool.Exec(ctx, updateTaskRunStateSQL, toState, runID, fromState, result, expectedVersion)
	return tag.RowsAffected() == 1, err
}
