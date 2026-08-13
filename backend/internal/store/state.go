package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

const updateTaskRunStateSQL = `
UPDATE task_runs
SET state = $1, result = $4, state_version = state_version + 1, updated_at = now()
WHERE id = $2 AND state_version = $3
`

// UpdateTaskRunState performs an optimistic state transition. A false result
// means another writer changed the run since expectedVersion was read.
func UpdateTaskRunState(ctx context.Context, pool *pgxpool.Pool, runID, state string, expectedVersion int64, result []byte) (bool, error) {
	tag, err := pool.Exec(ctx, updateTaskRunStateSQL, state, runID, expectedVersion, result)
	return tag.RowsAffected() == 1, err
}
