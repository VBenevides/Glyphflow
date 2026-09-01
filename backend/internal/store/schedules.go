package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5"
)

type ScheduleDefinition struct {
	ID, Name, TaskID, Expression, Timezone           string
	Enabled                                          bool
	NextFireAt                                       *time.Time
	MisfirePolicy, ConcurrencyPolicy                 string
	CatchupLimit, DeadlineSeconds, MaxConcurrentRuns int
}

type ScheduleRecord struct {
	ID, Name, TaskID, Expression, Timezone           string
	Enabled                                          bool
	NextFireAt                                       *time.Time
	State, MisfirePolicy, ConcurrencyPolicy          string
	CatchupLimit, DeadlineSeconds, MaxConcurrentRuns int
	ActiveVersion                                    int
}

type ScheduleProjectionResource struct {
	ID, Name, Kind string
}

type ScheduleProjectionInput struct {
	ScheduleID, ScheduleName, ScheduleVersionID string
	TaskID, TaskName, TaskVersionID             string
	Expression, Timezone                        string
	RunnerPoolID, RunnerPoolName                string
	PinnedRunnerID, PinnedRunnerName            string
	DurationSeconds                             int
	Resources                                   []ScheduleProjectionResource
}

type ScheduleRepository interface {
	List(context.Context) ([]ScheduleRecord, error)
	Find(context.Context, string) (ScheduleRecord, bool, error)
	Create(context.Context, ScheduleDefinition) (ScheduleRecord, error)
	Update(context.Context, string, ScheduleDefinition) (ScheduleRecord, error)
	SetEnabled(context.Context, string, bool) (ScheduleRecord, bool, error)
	Delete(context.Context, string) (bool, error)
	CreateDueRun(context.Context, time.Time, func(DueScheduleRecord) (time.Time, error)) (string, bool, error)
}

type ScheduleProjectionRepository interface {
	ListScheduleProjection(context.Context) ([]ScheduleProjectionInput, error)
}

type DueScheduleRecord struct {
	ID, TaskID, TaskVersionID, ScheduleVersionID                 string
	Expression, Timezone, MisfirePolicy, ConcurrencyPolicy       string
	NextFireAt                                                   time.Time
	CatchupLimit, DeadlineSeconds, MaxConcurrentRuns, ActiveRuns int
}

type ScheduleStore struct {
	pool            database
	storagePressure func(context.Context) (platform.StoragePressure, error)
}

const (
	defaultStartDeadlineSeconds = 60
	minimumStartDeadlineSeconds = 30
)

func NewScheduleRepository(pool any) *ScheduleStore {
	db, _ := databaseFrom(pool)
	return &ScheduleStore{pool: db}
}

func (s *ScheduleStore) SetStoragePressureProvider(provider func(context.Context) (platform.StoragePressure, error)) {
	s.storagePressure = provider
}

const scheduleQuery = `SELECT s.id, s.name, s.task_id, s.enabled, s.next_fire_at, sv.version, sv.expression, sv.timezone, sv.misfire_policy, sv.catchup_limit, sv.start_deadline_seconds, sv.concurrency_policy, sv.max_concurrent_runs FROM schedules s JOIN schedule_versions sv ON sv.id = s.current_version_id`

func (s *ScheduleStore) List(ctx context.Context) ([]ScheduleRecord, error) {
	rows, err := s.pool.Query(ctx, scheduleQuery+` ORDER BY lower(s.name), s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ScheduleRecord{}
	for rows.Next() {
		item, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ScheduleStore) Find(ctx context.Context, id string) (ScheduleRecord, bool, error) {
	item, err := scanSchedule(s.pool.QueryRow(ctx, scheduleQuery+` WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ScheduleRecord{}, false, nil
	}
	return item, err == nil, err
}

func (s *ScheduleStore) ListScheduleProjection(ctx context.Context) ([]ScheduleProjectionInput, error) {
	rows, err := s.pool.Query(ctx, `SELECT s.id, s.name, sv.id, s.task_id, t.name, tv.id, sv.expression, sv.timezone, tv.runner_pool_id, rp.name, COALESCE(tv.pinned_runner_id, ''), COALESCE(pr.name, ''), tv.duration_seconds, resource.id, resource.name, resource.kind FROM schedules s JOIN schedule_versions sv ON sv.id = s.current_version_id JOIN tasks t ON t.id = s.task_id AND t.enabled AND NOT t.is_deleted JOIN task_versions tv ON tv.id = sv.task_version_id AND tv.task_id = sv.task_id JOIN runner_pools rp ON rp.id = tv.runner_pool_id LEFT JOIN runners pr ON pr.id = tv.pinned_runner_id LEFT JOIN task_resource_requirements req ON req.task_version_id = tv.id LEFT JOIN resources resource ON resource.id = req.resource_id WHERE s.enabled ORDER BY lower(s.name), s.id, resource.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ScheduleProjectionInput{}
	for rows.Next() {
		var item ScheduleProjectionInput
		var resourceID, resourceName, resourceKind *string
		if err := rows.Scan(&item.ScheduleID, &item.ScheduleName, &item.ScheduleVersionID, &item.TaskID, &item.TaskName, &item.TaskVersionID, &item.Expression, &item.Timezone, &item.RunnerPoolID, &item.RunnerPoolName, &item.PinnedRunnerID, &item.PinnedRunnerName, &item.DurationSeconds, &resourceID, &resourceName, &resourceKind); err != nil {
			return nil, err
		}
		index := len(items) - 1
		if index < 0 || items[index].ScheduleID != item.ScheduleID {
			items = append(items, item)
			index++
		}
		if resourceID != nil && *resourceID != "" {
			items[index].Resources = append(items[index].Resources, ScheduleProjectionResource{ID: *resourceID, Name: valueOrID(resourceName, *resourceID), Kind: valueOrEmpty(resourceKind)})
		}
	}
	return items, rows.Err()
}

func valueOrID(value *string, id string) string {
	if value != nil && *value != "" {
		return *value
	}
	return id
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func scanSchedule(row interface{ Scan(...any) error }) (ScheduleRecord, error) {
	var item ScheduleRecord
	if err := row.Scan(&item.ID, &item.Name, &item.TaskID, &item.Enabled, &item.NextFireAt, &item.ActiveVersion, &item.Expression, &item.Timezone, &item.MisfirePolicy, &item.CatchupLimit, &item.DeadlineSeconds, &item.ConcurrencyPolicy, &item.MaxConcurrentRuns); err != nil {
		return ScheduleRecord{}, err
	}
	if item.Enabled {
		item.State = "ACTIVE"
	} else {
		item.State = "DISABLED"
	}
	return item, nil
}

func (s *ScheduleStore) Create(ctx context.Context, definition ScheduleDefinition) (ScheduleRecord, error) {
	definition = normalizeScheduleDefinition(definition)
	if err := validateScheduleDefinition(definition); err != nil {
		return ScheduleRecord{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ScheduleRecord{}, err
	}
	defer tx.Rollback(ctx)
	var taskVersionID string
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM tasks WHERE id = $1 AND NOT is_deleted FOR SHARE`, definition.TaskID).Scan(&taskVersionID); err != nil {
		return ScheduleRecord{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schedules (id, task_id, name, enabled, next_fire_at) VALUES ($1, $2, $3, $4, $5)`, definition.ID, definition.TaskID, definition.Name, definition.Enabled, definition.NextFireAt); err != nil {
		return ScheduleRecord{}, err
	}
	if err := insertScheduleVersion(ctx, tx, definition.ID, definition.TaskID, 1, taskVersionID, definition); err != nil {
		return ScheduleRecord{}, err
	}
	if err := recordGlobalVariableReferences(ctx, tx, "schedule_version", definition.ID+"-v1", definition.Expression, definition.Timezone, definition.Name); err != nil {
		return ScheduleRecord{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE schedules SET current_version_id = $2, updated_at = now() WHERE id = $1`, definition.ID, definition.ID+"-v1"); err != nil {
		return ScheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScheduleRecord{}, err
	}
	item, found, err := s.Find(ctx, definition.ID)
	if err != nil {
		return ScheduleRecord{}, err
	}
	if !found {
		return ScheduleRecord{}, errors.New("schedule was not created")
	}
	return item, nil
}

func (s *ScheduleStore) Update(ctx context.Context, id string, definition ScheduleDefinition) (ScheduleRecord, error) {
	definition = normalizeScheduleDefinition(definition)
	definition.ID = id
	if err := validateScheduleDefinition(definition); err != nil {
		return ScheduleRecord{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ScheduleRecord{}, err
	}
	defer tx.Rollback(ctx)
	var taskID, taskVersionID string
	if err := tx.QueryRow(ctx, `SELECT task_id FROM schedules WHERE id = $1 FOR UPDATE`, id).Scan(&taskID); err != nil {
		return ScheduleRecord{}, err
	}
	if definition.TaskID == "" {
		definition.TaskID = taskID
	}
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM tasks WHERE id = $1 AND NOT is_deleted FOR SHARE`, definition.TaskID).Scan(&taskVersionID); err != nil {
		return ScheduleRecord{}, err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM schedule_versions WHERE schedule_id = $1`, id).Scan(&version); err != nil {
		return ScheduleRecord{}, err
	}
	if err := insertScheduleVersion(ctx, tx, id, definition.TaskID, version, taskVersionID, definition); err != nil {
		return ScheduleRecord{}, err
	}
	versionID := id + "-v" + strconv.Itoa(version)
	if err := recordGlobalVariableReferences(ctx, tx, "schedule_version", versionID, definition.Expression, definition.Timezone, definition.Name); err != nil {
		return ScheduleRecord{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE schedules SET task_id = $2, name = $3, enabled = $4, current_version_id = $5, next_fire_at = $6, updated_at = now() WHERE id = $1`, id, definition.TaskID, definition.Name, definition.Enabled, versionID, definition.NextFireAt); err != nil {
		return ScheduleRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScheduleRecord{}, err
	}
	item, found, err := s.Find(ctx, id)
	if err != nil {
		return ScheduleRecord{}, err
	}
	if !found {
		return ScheduleRecord{}, errors.New("schedule was not found")
	}
	return item, nil
}

func (s *ScheduleStore) Delete(ctx context.Context, id string) (bool, error) {
	result, err := s.pool.Exec(ctx, `DELETE FROM schedules WHERE id = $1`, id)
	return result.RowsAffected() > 0, err
}

func (s *ScheduleStore) SetEnabled(ctx context.Context, id string, enabled bool) (ScheduleRecord, bool, error) {
	result, err := s.pool.Exec(ctx, `UPDATE schedules SET enabled = $2, updated_at = now() WHERE id = $1`, id, enabled)
	if err != nil || result.RowsAffected() == 0 {
		return ScheduleRecord{}, result.RowsAffected() > 0, err
	}
	item, found, err := s.Find(ctx, id)
	return item, found, err
}

func (s *ScheduleStore) CreateDueRun(ctx context.Context, now time.Time, next func(DueScheduleRecord) (time.Time, error)) (string, bool, error) {
	if next == nil {
		return "", false, errors.New("schedule next-fire function is required")
	}
	if s.storagePressure != nil {
		pressure, err := s.storagePressure(ctx)
		if err != nil {
			return "", false, err
		}
		if pressure.State == platform.StorageUnavailable {
			return "", false, ErrStorageUnavailable
		}
		if pressure.RejectNewRuns() {
			return "", false, ErrStorageExhausted
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)
	var due DueScheduleRecord
	var storedNext *time.Time
	err = tx.QueryRow(ctx, `SELECT s.id, s.task_id, sv.task_version_id, s.current_version_id, sv.expression, sv.timezone, sv.misfire_policy, sv.catchup_limit, sv.start_deadline_seconds, sv.concurrency_policy, sv.max_concurrent_runs, COALESCE((SELECT count(*) FROM runs active WHERE active.schedule_version_id = s.current_version_id AND active.state IN ('WAITING','RUNNING','RETRY_WAIT','CANCELLING')), 0), s.next_fire_at FROM schedules s JOIN schedule_versions sv ON sv.id = s.current_version_id WHERE s.enabled AND (s.next_fire_at IS NULL OR s.next_fire_at <= $1) AND NOT ((sv.concurrency_policy = 'ALLOW' AND sv.max_concurrent_runs > 0 AND COALESCE((SELECT count(*) FROM runs active WHERE active.schedule_version_id = s.current_version_id AND active.state IN ('WAITING','RUNNING','RETRY_WAIT','CANCELLING')), 0) >= sv.max_concurrent_runs) OR (sv.concurrency_policy = 'QUEUE' AND COALESCE((SELECT count(*) FROM runs active WHERE active.schedule_version_id = s.current_version_id AND active.state IN ('WAITING','RUNNING','RETRY_WAIT','CANCELLING')), 0) > 0)) ORDER BY (s.next_fire_at IS NULL), s.next_fire_at, s.id FOR UPDATE OF s SKIP LOCKED LIMIT 1`, now).Scan(&due.ID, &due.TaskID, &due.TaskVersionID, &due.ScheduleVersionID, &due.Expression, &due.Timezone, &due.MisfirePolicy, &due.CatchupLimit, &due.DeadlineSeconds, &due.ConcurrencyPolicy, &due.MaxConcurrentRuns, &due.ActiveRuns, &storedNext)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	variables, err := loadGlobalVariables(ctx, tx)
	if err != nil {
		return "", false, err
	}
	resolvedGlobals, err := json.Marshal(variables)
	if err != nil {
		return "", false, err
	}
	due.Expression, err = platform.ResolveGlobalVariables(due.Expression, variables)
	if err != nil {
		return "", false, err
	}
	due.Timezone, err = platform.ResolveGlobalVariables(due.Timezone, variables)
	if err != nil {
		return "", false, err
	}
	initialized := storedNext == nil
	if initialized {
		due.NextFireAt = now
	} else {
		due.NextFireAt = storedNext.UTC()
	}
	nextFire, err := next(due)
	if err != nil {
		return "", false, err
	}
	if !nextFire.After(due.NextFireAt) {
		return "", false, errors.New("schedule next fire did not advance")
	}
	if initialized {
		if _, err := tx.Exec(ctx, `UPDATE schedules SET next_fire_at = $2, updated_at = now() WHERE id = $1`, due.ID, nextFire); err != nil {
			return "", false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", false, err
		}
		return "", true, nil
	}
	if due.ConcurrencyPolicy == "SKIP" && due.ActiveRuns > 0 {
		if _, err := tx.Exec(ctx, `UPDATE schedules SET next_fire_at = $2, updated_at = now() WHERE id = $1`, due.ID, nextFire); err != nil {
			return "", false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", false, err
		}
		return "", true, nil
	}
	if due.ConcurrencyPolicy == "ALLOW" && due.MaxConcurrentRuns > 0 && due.ActiveRuns >= due.MaxConcurrentRuns {
		return "", false, nil
	}
	if due.ConcurrencyPolicy == "QUEUE" && due.ActiveRuns > 0 {
		return "", false, nil
	}
	if due.ConcurrencyPolicy == "REPLACE" && due.ActiveRuns > 0 {
		if _, err := tx.Exec(ctx, `UPDATE runs SET state = 'CANCELLING', cancellation_requested_at = COALESCE(cancellation_requested_at, now()), cancellation_reason = 'schedule replacement', state_version = state_version + 1, updated_at = now() WHERE schedule_version_id = $1 AND state IN ('WAITING','DISPATCHED','RUNNING','RETRY_WAIT')`, due.ScheduleVersionID); err != nil {
			return "", false, err
		}
	}
	occurrence, nextFire, err := chooseDueOccurrence(due, now, nextFire, next)
	if err != nil {
		return "", false, err
	}
	if occurrence.IsZero() {
		if _, err := tx.Exec(ctx, `UPDATE schedules SET next_fire_at = $2, updated_at = now() WHERE id = $1`, due.ID, nextFire); err != nil {
			return "", false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return "", false, err
		}
		return "", true, nil
	}
	runID, err := NewRunID()
	if err != nil {
		return "", false, err
	}
	idempotencyKey := due.ID + ":" + occurrence.UTC().Format(time.RFC3339Nano)
	if nextFire.IsZero() {
		return "", false, errors.New("schedule next fire is empty")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runs (id, task_id, task_version_id, schedule_version_id, trigger_type, scheduled_for, resolved_global_variables, start_deadline_at, state, idempotency_key) VALUES ($1, $2, $3, $4, 'SCHEDULE', $5, $6::jsonb, $7, 'WAITING', $8) ON CONFLICT (idempotency_key) DO NOTHING`, runID, due.TaskID, due.TaskVersionID, due.ScheduleVersionID, occurrence, resolvedGlobals, deadlineValue(occurrence, due.DeadlineSeconds), idempotencyKey); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE schedules SET next_fire_at = $2, updated_at = now() WHERE id = $1`, due.ID, nextFire); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return runID, true, nil
}

func deadlineValue(occurrence time.Time, seconds int) any {
	if seconds <= 0 {
		return occurrence.Add(defaultStartDeadlineSeconds * time.Second)
	}
	return occurrence.Add(time.Duration(seconds) * time.Second)
}

func chooseDueOccurrence(due DueScheduleRecord, now, nextFire time.Time, next func(DueScheduleRecord) (time.Time, error)) (time.Time, time.Time, error) {
	if nextFire.After(now) {
		return due.NextFireAt, nextFire, nil
	}
	missed := []time.Time{due.NextFireAt}
	current := nextFire
	for !current.After(now) && len(missed) < 1000 {
		missed = append(missed, current)
		current, err := next(DueScheduleRecord{Expression: due.Expression, Timezone: due.Timezone, NextFireAt: current})
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		if current.IsZero() {
			return time.Time{}, time.Time{}, errors.New("schedule next fire is empty")
		}
	}
	switch due.MisfirePolicy {
	case "SKIP_ALL", "FAIL_AND_ALERT":
		return time.Time{}, current, nil
	case "RUN_LATEST":
		return missed[len(missed)-1], current, nil
	case "RUN_UP_TO_N":
		limit := due.CatchupLimit
		if limit <= 0 || len(missed) <= limit {
			return missed[0], nextFire, nil
		}
		index := len(missed) - limit
		return missed[index], missed[index+1], nil
	default:
		return missed[0], nextFire, nil
	}
}

func normalizeScheduleDefinition(definition ScheduleDefinition) ScheduleDefinition {
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.Timezone == "" {
		definition.Timezone = "UTC"
	}
	if definition.MisfirePolicy == "" {
		definition.MisfirePolicy = "SKIP_ALL"
	}
	if definition.DeadlineSeconds == 0 {
		definition.DeadlineSeconds = defaultStartDeadlineSeconds
	}
	if definition.ConcurrencyPolicy == "" {
		definition.ConcurrencyPolicy = "ALLOW"
	}
	return definition
}

func validateScheduleDefinition(definition ScheduleDefinition) error {
	if definition.ID == "" || definition.Name == "" || definition.TaskID == "" || definition.Expression == "" {
		return errors.New("schedule id, task, name, and expression are required")
	}
	if _, err := platform.ScheduleLocation(definition.Timezone); err != nil && globalVariableReferencePattern.FindString(definition.Timezone) != definition.Timezone {
		return errors.New("schedule timezone is invalid")
	}
	if definition.DeadlineSeconds < minimumStartDeadlineSeconds {
		return errors.New("start deadline must be at least 30 seconds")
	}
	return nil
}

func insertScheduleVersion(ctx context.Context, tx databaseTx, scheduleID, taskID string, version int, taskVersionID string, definition ScheduleDefinition) error {
	_, err := tx.Exec(ctx, `INSERT INTO schedule_versions (id, schedule_id, task_id, version, task_version_id, expression, timezone, misfire_policy, catchup_limit, start_deadline_seconds, concurrency_policy, max_concurrent_runs) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, scheduleID+"-v"+strconv.Itoa(version), scheduleID, taskID, version, taskVersionID, definition.Expression, definition.Timezone, definition.MisfirePolicy, definition.CatchupLimit, definition.DeadlineSeconds, definition.ConcurrencyPolicy, definition.MaxConcurrentRuns)
	return err
}
