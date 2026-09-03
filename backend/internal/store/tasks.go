package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type TaskDefinition struct {
	ID, Name, Description, RunnerPoolID, PinnedRunnerID string
	Enabled                                             bool
	Command                                             []string
	DurationSeconds                                     int
	MaxOutputBytes                                      int64
	MaxAttempts                                         int
	InitialBackoffSeconds                               int
	MaxBackoffSeconds                                   int
	BackoffMultiplier                                   float64
	WorkingDirectory                                    string
	PlacementSelectors, Environment                     map[string]any
	SecretReferences                                    map[string]any
	ResourceIDs                                         []string
	RetryableExitCodes                                  []int
	RetryableTerminationReasons                         []string
	AmbiguityPolicy                                     string
	ExecutionSpecVersion                                int
	ExecutionSpecDigest                                 string
}

type TaskRecord struct {
	ID, CurrentVersionID, Name, RunnerPoolID, PinnedRunnerID string
	Enabled                                                  bool
	IsDeleted                                                bool
	ActiveVersion, DurationSeconds, MaxAttempts              int
	MaxOutputBytes                                           int64
	WorkingDirectory, ExecutionSpecDigest, AmbiguityPolicy   string
	Command                                                  []string
	PlacementSelectors, Environment, SecretReferences        map[string]any
	ResourceIDs                                              []string
	LatestRun                                                *RunRecord
}

type TaskVersionRecord struct {
	ID, RunnerPoolID, PinnedRunnerID, WorkingDirectory, AmbiguityPolicy, ExecutionSpecDigest string
	Version, DurationSeconds, MaxAttempts                                                    int
	MaxOutputBytes                                                                           int64
	Command                                                                                  []string
	ResourceIDs                                                                              []string
	CreatedAt                                                                                time.Time
}

type TaskRepository interface {
	List(context.Context, bool) ([]TaskRecord, error)
	Find(context.Context, string) (TaskRecord, bool, error)
	ListVersions(context.Context, string) ([]TaskVersionRecord, error)
	Create(context.Context, TaskDefinition) (TaskRecord, error)
	CreateVersion(context.Context, string, TaskDefinition) (TaskRecord, error)
	Delete(context.Context, string) (bool, error)
}

type TaskStore struct{ pool database }

func NewTaskRepository(pool any) *TaskStore {
	db, _ := databaseFrom(pool)
	return &TaskStore{pool: db}
}

func (s *TaskStore) List(ctx context.Context, archived bool) ([]TaskRecord, error) {
	filter := `NOT t.is_deleted`
	if archived {
		filter = `t.is_deleted`
	}
	rows, err := s.pool.Query(ctx, taskQuery+` WHERE `+filter+` ORDER BY lower(t.name), t.id`)
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
	item, err := scanTask(s.pool.QueryRow(ctx, taskQuery+` WHERE t.id = $1 AND NOT t.is_deleted`, id), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return TaskRecord{}, false, nil
	}
	return item, err == nil, err
}

func (s *TaskStore) ListVersions(ctx context.Context, taskID string) ([]TaskVersionRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT v.id, v.version, v.runner_pool_id, COALESCE(v.pinned_runner_id, ''), v.command, v.working_directory, v.duration_seconds, v.max_output_bytes, v.max_attempts, v.ambiguity_policy, v.execution_spec_digest, COALESCE((SELECT jsonb_agg(req.resource_id ORDER BY req.resource_id) FROM task_resource_requirements req WHERE req.task_version_id = v.id), '[]'::jsonb), v.created_at FROM task_versions v JOIN tasks t ON t.id = v.task_id WHERE v.task_id = $1 AND NOT t.is_deleted ORDER BY v.version DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []TaskVersionRecord{}
	for rows.Next() {
		var item TaskVersionRecord
		var command, resources []byte
		if err := rows.Scan(&item.ID, &item.Version, &item.RunnerPoolID, &item.PinnedRunnerID, &command, &item.WorkingDirectory, &item.DurationSeconds, &item.MaxOutputBytes, &item.MaxAttempts, &item.AmbiguityPolicy, &item.ExecutionSpecDigest, &resources, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(command, &item.Command); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(resources, &item.ResourceIDs); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

const taskQuery = `WITH latest_attempt AS (SELECT ea.run_id, ea.runner_id, ea.exit_code, ROW_NUMBER() OVER (PARTITION BY ea.run_id ORDER BY ea.attempt_number DESC) AS row_number FROM execution_attempts ea), latest_runs AS (SELECT r.id, r.task_id, t_run.name AS task_name, r.state, r.trigger_type, r.scheduled_for, COALESCE((SELECT MAX(attempt_number) FROM execution_attempts WHERE run_id = r.id), 1) AS attempt, latest_attempt.runner_id AS runner, latest_attempt.exit_code, COALESCE(ec.meaning, '') AS exit_code_meaning, ROW_NUMBER() OVER (PARTITION BY r.task_id ORDER BY r.created_at DESC, r.id DESC) AS row_number FROM runs r JOIN tasks t_run ON t_run.id = r.task_id LEFT JOIN latest_attempt ON latest_attempt.run_id = r.id AND latest_attempt.row_number = 1 LEFT JOIN exit_code ec ON ec.code = latest_attempt.exit_code) SELECT t.id, COALESCE(t.current_version_id, ''), t.name, t.enabled, COALESCE(v.version, 0), COALESCE(v.runner_pool_id, ''), COALESCE(v.pinned_runner_id, ''), COALESCE(v.command, '[]'::jsonb), COALESCE(v.working_directory, '.'), COALESCE(v.placement_selectors, '{}'::jsonb), COALESCE(v.environment, '{}'::jsonb), COALESCE(v.secret_references, '{}'::jsonb), COALESCE(v.duration_seconds, 0), COALESCE(v.max_output_bytes, 0), COALESCE(v.max_attempts, 1), COALESCE(v.ambiguity_policy, ''), COALESCE(v.execution_spec_digest, ''), COALESCE((SELECT jsonb_agg(req.resource_id ORDER BY req.resource_id) FROM task_resource_requirements req WHERE req.task_version_id = v.id), '[]'::jsonb), COALESCE(latest_run.id, ''), COALESCE(latest_run.task_id, ''), COALESCE(latest_run.task_name, ''), COALESCE(latest_run.state, ''), COALESCE(latest_run.attempt, 0), latest_run.exit_code, COALESCE(latest_run.exit_code_meaning, ''), COALESCE(latest_run.runner, ''), COALESCE(latest_run.trigger_type, ''), COALESCE(latest_run.scheduled_for, 'epoch'::timestamptz), t.is_deleted FROM tasks t LEFT JOIN task_versions v ON v.id = t.current_version_id LEFT JOIN latest_runs latest_run ON latest_run.task_id = t.id AND latest_run.row_number = 1`

type rowScanner interface{ Scan(...any) error }

func scanTask(row rowScanner, _ ...string) (TaskRecord, error) {
	var item TaskRecord
	var command, selectors, environment, secrets, resources []byte
	var latestRunID, latestTaskID, latestTaskName, latestState, latestMeaning, latestRunner, latestTrigger string
	var latestAttempt int
	var latestExitCode *int
	var latestScheduledFor time.Time
	if err := row.Scan(&item.ID, &item.CurrentVersionID, &item.Name, &item.Enabled, &item.ActiveVersion, &item.RunnerPoolID, &item.PinnedRunnerID, &command, &item.WorkingDirectory, &selectors, &environment, &secrets, &item.DurationSeconds, &item.MaxOutputBytes, &item.MaxAttempts, &item.AmbiguityPolicy, &item.ExecutionSpecDigest, &resources, &latestRunID, &latestTaskID, &latestTaskName, &latestState, &latestAttempt, &latestExitCode, &latestMeaning, &latestRunner, &latestTrigger, &latestScheduledFor, &item.IsDeleted); err != nil {
		return TaskRecord{}, err
	}
	if err := json.Unmarshal(command, &item.Command); err != nil {
		return TaskRecord{}, err
	}
	if err := json.Unmarshal(selectors, &item.PlacementSelectors); err != nil {
		return TaskRecord{}, err
	}
	if err := json.Unmarshal(environment, &item.Environment); err != nil {
		return TaskRecord{}, err
	}
	if err := json.Unmarshal(secrets, &item.SecretReferences); err != nil {
		return TaskRecord{}, err
	}
	if err := json.Unmarshal(resources, &item.ResourceIDs); err != nil {
		return TaskRecord{}, err
	}
	if latestRunID != "" {
		item.LatestRun = &RunRecord{ID: latestRunID, TaskID: latestTaskID, TaskName: latestTaskName, State: latestState, Attempt: latestAttempt, ExitCode: latestExitCode, ExitCodeMeaning: latestMeaning, Runner: latestRunner, TriggerType: latestTrigger, ScheduledFor: latestScheduledFor}
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
	if err := tx.QueryRow(ctx, `SELECT id FROM tasks WHERE id = $1 AND NOT is_deleted FOR UPDATE`, taskID).Scan(new(string)); err != nil {
		return TaskRecord{}, err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM task_versions WHERE task_id = $1`, taskID).Scan(&version); err != nil {
		return TaskRecord{}, err
	}
	previous, err := loadPreviousTaskDefinition(ctx, tx, taskID)
	if err != nil {
		return TaskRecord{}, err
	}
	requested := definition
	definition = normalizeTaskDefinition(definition)
	if requested.Name == "" {
		definition.Name = previous.Name
	}
	if requested.RunnerPoolID == "" {
		definition.RunnerPoolID = previous.RunnerPoolID
	}
	if requested.PinnedRunnerID == "" {
		definition.PinnedRunnerID = previous.PinnedRunnerID
	}
	if len(requested.Command) == 0 {
		definition.Command = previous.Command
	}
	if requested.WorkingDirectory == "" {
		definition.WorkingDirectory = previous.WorkingDirectory
	}
	if requested.PlacementSelectors == nil {
		definition.PlacementSelectors = previous.PlacementSelectors
	}
	if requested.Environment == nil {
		definition.Environment = previous.Environment
	}
	if requested.SecretReferences == nil {
		definition.SecretReferences = previous.SecretReferences
	}
	if requested.ResourceIDs == nil {
		definition.ResourceIDs = previous.ResourceIDs
	}
	if requested.DurationSeconds <= 0 {
		definition.DurationSeconds = previous.DurationSeconds
	}
	if requested.MaxOutputBytes <= 0 {
		definition.MaxOutputBytes = previous.MaxOutputBytes
	}
	if requested.MaxAttempts <= 0 {
		definition.MaxAttempts = previous.MaxAttempts
	}
	if requested.InitialBackoffSeconds == 0 {
		definition.InitialBackoffSeconds = previous.InitialBackoffSeconds
	}
	if requested.MaxBackoffSeconds == 0 {
		definition.MaxBackoffSeconds = previous.MaxBackoffSeconds
	}
	if requested.BackoffMultiplier == 0 {
		definition.BackoffMultiplier = previous.BackoffMultiplier
	}
	if requested.RetryableExitCodes == nil {
		definition.RetryableExitCodes = previous.RetryableExitCodes
	}
	if requested.RetryableTerminationReasons == nil {
		definition.RetryableTerminationReasons = previous.RetryableTerminationReasons
	}
	if requested.AmbiguityPolicy == "" {
		definition.AmbiguityPolicy = previous.AmbiguityPolicy
	}
	if requested.ExecutionSpecVersion <= 0 {
		definition.ExecutionSpecVersion = previous.ExecutionSpecVersion
	}
	definition.ExecutionSpecDigest = ""
	definition = normalizeTaskDefinition(definition)
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

func loadPreviousTaskDefinition(ctx context.Context, tx databaseTx, taskID string) (TaskDefinition, error) {
	var previous TaskDefinition
	var command, selectors, environment, secrets, resources, exitCodes, reasons []byte
	err := tx.QueryRow(ctx, `SELECT t.name, v.runner_pool_id, COALESCE(v.pinned_runner_id, ''), v.command, v.working_directory, v.placement_selectors, v.environment, v.secret_references, v.duration_seconds, v.max_output_bytes, v.max_attempts, v.initial_backoff_seconds, v.max_backoff_seconds, v.backoff_multiplier, v.retryable_exit_codes, v.retryable_termination_reasons, v.ambiguity_policy, v.execution_spec_version, COALESCE((SELECT jsonb_agg(req.resource_id ORDER BY req.resource_id) FROM task_resource_requirements req WHERE req.task_version_id = v.id), '[]'::jsonb) FROM task_versions v JOIN tasks t ON t.id = v.task_id WHERE v.task_id = $1 ORDER BY v.version DESC LIMIT 1`, taskID).Scan(&previous.Name, &previous.RunnerPoolID, &previous.PinnedRunnerID, &command, &previous.WorkingDirectory, &selectors, &environment, &secrets, &previous.DurationSeconds, &previous.MaxOutputBytes, &previous.MaxAttempts, &previous.InitialBackoffSeconds, &previous.MaxBackoffSeconds, &previous.BackoffMultiplier, &exitCodes, &reasons, &previous.AmbiguityPolicy, &previous.ExecutionSpecVersion, &resources)
	if err != nil {
		return TaskDefinition{}, err
	}
	for _, value := range []struct {
		raw  []byte
		dest any
	}{
		{command, &previous.Command}, {selectors, &previous.PlacementSelectors}, {environment, &previous.Environment},
		{secrets, &previous.SecretReferences}, {exitCodes, &previous.RetryableExitCodes}, {reasons, &previous.RetryableTerminationReasons}, {resources, &previous.ResourceIDs},
	} {
		if err := json.Unmarshal(value.raw, value.dest); err != nil {
			return TaskDefinition{}, err
		}
	}
	return previous, nil
}

func (s *TaskStore) Delete(ctx context.Context, id string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE tasks SET is_deleted = true, enabled = false, updated_at = now() WHERE id = $1 AND NOT is_deleted`, id)
	if err != nil || result.RowsAffected() == 0 {
		return result.RowsAffected() > 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE schedules SET enabled = false, updated_at = now() WHERE task_id = $1`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE runs SET state = CASE WHEN state IN ('WAITING','RETRY_WAIT') THEN 'CANCELLED' ELSE 'CANCELLING' END, cancellation_requested_at = COALESCE(cancellation_requested_at, now()), cancellation_reason = 'task deleted', completed_at = CASE WHEN state IN ('WAITING','RETRY_WAIT') THEN now() ELSE completed_at END, state_version = state_version + 1, updated_at = now() WHERE task_id = $1 AND state IN ('WAITING','DISPATCHED','RUNNING','RETRY_WAIT')`, id); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func normalizeTaskDefinition(definition TaskDefinition) TaskDefinition {
	normalizeTaskScalars(&definition)
	normalizeTaskMaps(&definition)
	if definition.ResourceIDs != nil {
		definition.ResourceIDs = normalizeResourceIDs(definition.ResourceIDs)
	}
	if definition.ExecutionSpecDigest == "" {
		raw, _ := json.Marshal(definition)
		digest := sha256.Sum256(raw)
		definition.ExecutionSpecDigest = hex.EncodeToString(digest[:])
	}
	return definition
}

func normalizeTaskScalars(definition *TaskDefinition) {
	if definition.DurationSeconds <= 0 {
		definition.DurationSeconds = 60
	}
	if definition.WorkingDirectory == "" {
		definition.WorkingDirectory = "."
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
}

func normalizeTaskMaps(definition *TaskDefinition) {
	if definition.PlacementSelectors == nil {
		definition.PlacementSelectors = map[string]any{}
	}
	if definition.Environment == nil {
		definition.Environment = map[string]any{}
	}
	if definition.SecretReferences == nil {
		definition.SecretReferences = map[string]any{}
	}
}

func normalizeResourceIDs(resourceIDs []string) []string {
	seen := make(map[string]struct{}, len(resourceIDs))
	resources := make([]string, 0, len(resourceIDs))
	for _, resourceID := range resourceIDs {
		resourceID = strings.TrimSpace(resourceID)
		if resourceID == "" {
			continue
		}
		if _, exists := seen[resourceID]; exists {
			continue
		}
		seen[resourceID] = struct{}{}
		resources = append(resources, resourceID)
	}
	sort.Strings(resources)
	return resources
}

func insertTaskVersion(ctx context.Context, tx databaseTx, taskID string, version int, definition TaskDefinition) error {
	var poolExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM runner_pools WHERE id = $1 AND NOT is_deleted)`, definition.RunnerPoolID).Scan(&poolExists); err != nil {
		return err
	}
	if !poolExists {
		return errors.New("runner pool not found")
	}
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
	if _, err = tx.Exec(ctx, `INSERT INTO task_versions (id, task_id, version, runner_pool_id, pinned_runner_id, placement_selectors, command, working_directory, environment, secret_references, duration_seconds, max_output_bytes, max_attempts, initial_backoff_seconds, max_backoff_seconds, backoff_multiplier, retryable_exit_codes, retryable_termination_reasons, ambiguity_policy, execution_spec_version, execution_spec_digest) VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6::jsonb, $7::jsonb, $8, $9::jsonb, $10::jsonb, $11, $12, $13, $14, $15, $16, $17::jsonb, $18::jsonb, $19, $20, $21)`, versionID, taskID, version, definition.RunnerPoolID, definition.PinnedRunnerID, placement, command, definition.WorkingDirectory, environment, secrets, definition.DurationSeconds, definition.MaxOutputBytes, definition.MaxAttempts, definition.InitialBackoffSeconds, definition.MaxBackoffSeconds, definition.BackoffMultiplier, exitCodes, reasons, definition.AmbiguityPolicy, definition.ExecutionSpecVersion, definition.ExecutionSpecDigest); err != nil {
		return err
	}
	for _, resourceID := range definition.ResourceIDs {
		if _, err := tx.Exec(ctx, `INSERT INTO task_resource_requirements (task_version_id, resource_id) VALUES ($1, $2)`, versionID, resourceID); err != nil {
			return err
		}
	}
	return recordGlobalVariableReferences(ctx, tx, "task_version", versionID, definition.Command, definition.WorkingDirectory, definition.Environment, definition.PlacementSelectors)
}

var globalVariableReferencePattern = regexp.MustCompile(`\$ENV:([A-Z_][A-Z0-9_]*)`)

func recordGlobalVariableReferences(ctx context.Context, tx databaseTx, ownerType, ownerID string, values ...any) error {
	seen := map[string]bool{}
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		for _, match := range globalVariableReferencePattern.FindAllStringSubmatch(string(raw), -1) {
			name := match[1]
			if seen[strings.ToLower(name)] {
				continue
			}
			seen[strings.ToLower(name)] = true
			var id string
			if err := tx.QueryRow(ctx, `SELECT id FROM global_variables WHERE lower(name) = lower($1)`, name).Scan(&id); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return errors.New("global variable is not defined: " + name)
				}
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO global_variable_references (variable_id, owner_type, owner_id) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, ownerType, ownerID); err != nil {
				return err
			}
		}
	}
	return nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
