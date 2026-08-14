package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TaskDefinition struct {
	ID, Name, Description, RunnerPoolID string
	Enabled                             bool
	Command                             []string
	TimeoutSeconds                      int
	MaxOutputBytes                      int64
	MaxAttempts                         int
	InitialBackoffSeconds               int
	MaxBackoffSeconds                   int
	BackoffMultiplier                   float64
	WorkingDirectory                    string
	PlacementSelectors, Environment     map[string]any
	SecretReferences                    map[string]any
	RetryableExitCodes                  []int
	RetryableTerminationReasons         []string
	AmbiguityPolicy                     string
	ExecutionSpecVersion                int
	ExecutionSpecDigest                 string
}

type TaskRecord struct {
	ID, CurrentVersionID, Name, RunnerPoolID string
	Enabled                                  bool
	ActiveVersion, TimeoutSeconds            int
	Command                                  []string
}

type TaskRepository interface {
	List(context.Context) ([]TaskRecord, error)
	Find(context.Context, string) (TaskRecord, bool, error)
	Create(context.Context, TaskDefinition) (TaskRecord, error)
	CreateVersion(context.Context, string, TaskDefinition) (TaskRecord, error)
	Delete(context.Context, string) (bool, error)
}

type TaskStore struct{ pool *pgxpool.Pool }

func NewTaskRepository(pool *pgxpool.Pool) *TaskStore { return &TaskStore{pool: pool} }

func (s *TaskStore) List(ctx context.Context) ([]TaskRecord, error) {
	rows, err := s.pool.Query(ctx, taskQuery+` ORDER BY lower(t.name), t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TaskRecord{}
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *TaskStore) Find(ctx context.Context, id string) (TaskRecord, bool, error) {
	item, err := scanTask(s.pool.QueryRow(ctx, taskQuery+` WHERE t.id = $1`, id), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return TaskRecord{}, false, nil
	}
	return item, err == nil, err
}

const taskQuery = `SELECT t.id, COALESCE(t.current_version_id, ''), t.name, t.enabled, COALESCE(v.version, 0), COALESCE(v.runner_pool_id, ''), COALESCE(v.command, '[]'::jsonb), COALESCE(v.timeout_seconds, 0) FROM tasks t LEFT JOIN task_versions v ON v.id = t.current_version_id`

type rowScanner interface{ Scan(...any) error }

func scanTask(row rowScanner, _ ...string) (TaskRecord, error) {
	var item TaskRecord
	var command []byte
	if err := row.Scan(&item.ID, &item.CurrentVersionID, &item.Name, &item.Enabled, &item.ActiveVersion, &item.RunnerPoolID, &command, &item.TimeoutSeconds); err != nil {
		return TaskRecord{}, err
	}
	if err := json.Unmarshal(command, &item.Command); err != nil {
		return TaskRecord{}, err
	}
	return item, nil
}

func (s *TaskStore) Create(ctx context.Context, definition TaskDefinition) (TaskRecord, error) {
	definition = normalizeTaskDefinition(definition)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TaskRecord{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO tasks (id, name, description, enabled) VALUES ($1, $2, $3, $4)`, definition.ID, definition.Name, definition.Description, definition.Enabled); err != nil {
		return TaskRecord{}, err
	}
	if err := insertTaskVersion(ctx, tx, definition.ID, 1, definition); err != nil {
		return TaskRecord{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET current_version_id = $2, updated_at = now() WHERE id = $1`, definition.ID, definition.ID+"-v1"); err != nil {
		return TaskRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TaskRecord{}, err
	}
	item, found, err := s.Find(ctx, definition.ID)
	if err != nil {
		return TaskRecord{}, err
	}
	if !found {
		return TaskRecord{}, errors.New("task was not created")
	}
	return item, nil
}

func (s *TaskStore) CreateVersion(ctx context.Context, taskID string, definition TaskDefinition) (TaskRecord, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TaskRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `SELECT id FROM tasks WHERE id = $1 FOR UPDATE`, taskID).Scan(new(string)); err != nil {
		return TaskRecord{}, err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM task_versions WHERE task_id = $1`, taskID).Scan(&version); err != nil {
		return TaskRecord{}, err
	}
	var runnerPool string
	var timeout int
	if err := tx.QueryRow(ctx, `SELECT runner_pool_id, timeout_seconds FROM task_versions WHERE task_id = $1 ORDER BY version DESC LIMIT 1`, taskID).Scan(&runnerPool, &timeout); err != nil {
		return TaskRecord{}, err
	}
	definition = normalizeTaskDefinition(definition)
	if definition.RunnerPoolID == "" {
		definition.RunnerPoolID = runnerPool
	}
	if definition.TimeoutSeconds == 60 {
		definition.TimeoutSeconds = timeout
	}
	if err := insertTaskVersion(ctx, tx, taskID, version, definition); err != nil {
		return TaskRecord{}, err
	}
	versionID := taskID + "-v" + strconv.Itoa(version)
	if _, err := tx.Exec(ctx, `UPDATE tasks SET name = COALESCE(NULLIF($2, ''), name), current_version_id = $3, updated_at = now() WHERE id = $1`, taskID, definition.Name, versionID); err != nil {
		return TaskRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TaskRecord{}, err
	}
	item, found, err := s.Find(ctx, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	if !found {
		return TaskRecord{}, errors.New("task was not found")
	}
	return item, nil
}

func (s *TaskStore) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	return result.RowsAffected() > 0, err
}

func normalizeTaskDefinition(definition TaskDefinition) TaskDefinition {
	if definition.TimeoutSeconds <= 0 {
		definition.TimeoutSeconds = 60
	}
	if definition.MaxOutputBytes <= 0 {
		definition.MaxOutputBytes = 1 << 20
	}
	if definition.MaxAttempts <= 0 {
		definition.MaxAttempts = 1
	}
	if definition.MaxBackoffSeconds < definition.InitialBackoffSeconds {
		definition.MaxBackoffSeconds = maxInt(definition.InitialBackoffSeconds, 3600)
	}
	if definition.BackoffMultiplier < 1 {
		definition.BackoffMultiplier = 2
	}
	if definition.AmbiguityPolicy == "" {
		definition.AmbiguityPolicy = "REQUIRE_MANUAL_RECONCILIATION"
	}
	if definition.ExecutionSpecVersion <= 0 {
		definition.ExecutionSpecVersion = 1
	}
	if definition.PlacementSelectors == nil {
		definition.PlacementSelectors = map[string]any{}
	}
	if definition.Environment == nil {
		definition.Environment = map[string]any{}
	}
	if definition.SecretReferences == nil {
		definition.SecretReferences = map[string]any{}
	}
	if definition.ExecutionSpecDigest == "" {
		raw, _ := json.Marshal(definition)
		digest := sha256.Sum256(raw)
		definition.ExecutionSpecDigest = hex.EncodeToString(digest[:])
	}
	return definition
}

func insertTaskVersion(ctx context.Context, tx pgx.Tx, taskID string, version int, definition TaskDefinition) error {
	command, err := json.Marshal(definition.Command)
	if err != nil {
		return err
	}
	placement, err := json.Marshal(definition.PlacementSelectors)
	if err != nil {
		return err
	}
	environment, err := json.Marshal(definition.Environment)
	if err != nil {
		return err
	}
	secrets, err := json.Marshal(definition.SecretReferences)
	if err != nil {
		return err
	}
	exitCodes, err := json.Marshal(definition.RetryableExitCodes)
	if err != nil {
		return err
	}
	reasons, err := json.Marshal(definition.RetryableTerminationReasons)
	if err != nil {
		return err
	}
	versionID := taskID + "-v" + strconv.Itoa(version)
	_, err = tx.Exec(ctx, `INSERT INTO task_versions (id, task_id, version, runner_pool_id, placement_selectors, command, working_directory, environment, secret_references, timeout_seconds, max_output_bytes, max_attempts, initial_backoff_seconds, max_backoff_seconds, backoff_multiplier, retryable_exit_codes, retryable_termination_reasons, ambiguity_policy, execution_spec_version, execution_spec_digest) VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8::jsonb, $9::jsonb, $10, $11, $12, $13, $14, $15, $16::jsonb, $17::jsonb, $18, $19, $20)`, versionID, taskID, version, definition.RunnerPoolID, placement, command, definition.WorkingDirectory, environment, secrets, definition.TimeoutSeconds, definition.MaxOutputBytes, definition.MaxAttempts, definition.InitialBackoffSeconds, definition.MaxBackoffSeconds, definition.BackoffMultiplier, exitCodes, reasons, definition.AmbiguityPolicy, definition.ExecutionSpecVersion, definition.ExecutionSpecDigest)
	return err
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
