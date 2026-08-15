package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ScheduleDefinition struct {
	ID, Name, TaskID, Expression, Timezone string
	Enabled                                bool
	NextFireAt                             *time.Time
	MisfirePolicy, ConcurrencyPolicy       string
	CatchupLimit, DeadlineSeconds, MaxConcurrentRuns int
}

type ScheduleRecord struct {
	ID, Name, TaskID, Expression, Timezone string
	Enabled                                bool
	NextFireAt                             *time.Time
	State, MisfirePolicy, ConcurrencyPolicy string
	CatchupLimit, DeadlineSeconds, MaxConcurrentRuns int
	ActiveVersion                         int
}

type ScheduleRepository interface {
	List(context.Context) ([]ScheduleRecord, error)
	Find(context.Context, string) (ScheduleRecord, bool, error)
	Create(context.Context, ScheduleDefinition) (ScheduleRecord, error)
	Update(context.Context, string, ScheduleDefinition) (ScheduleRecord, error)
	Delete(context.Context, string) (bool, error)
	CreateDueRun(context.Context, time.Time, func(DueScheduleRecord) (time.Time, error)) (string, bool, error)
}

type DueScheduleRecord struct {
	ID, TaskID, TaskVersionID, ScheduleVersionID string
	Expression, Timezone                         string
	NextFireAt                                   time.Time
}

type ScheduleStore struct{ pool *pgxpool.Pool }

func NewScheduleRepository(pool *pgxpool.Pool) *ScheduleStore { return &ScheduleStore{pool: pool} }

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
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM tasks WHERE id = $1 FOR SHARE`, definition.TaskID).Scan(&taskVersionID); err != nil {
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
	if err := tx.QueryRow(ctx, `SELECT current_version_id FROM tasks WHERE id = $1 FOR SHARE`, definition.TaskID).Scan(&taskVersionID); err != nil {
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

func (s *ScheduleStore) CreateDueRun(ctx context.Context, now time.Time, next func(DueScheduleRecord) (time.Time, error)) (string, bool, error) {
	if next == nil {
		return "", false, errors.New("schedule next-fire function is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)
	var due DueScheduleRecord
	var storedNext *time.Time
	err = tx.QueryRow(ctx, `SELECT s.id, s.task_id, sv.task_version_id, s.current_version_id, sv.expression, sv.timezone, s.next_fire_at FROM schedules s JOIN schedule_versions sv ON sv.id = s.current_version_id WHERE s.enabled AND (s.next_fire_at IS NULL OR s.next_fire_at <= $1) ORDER BY COALESCE(s.next_fire_at, $1), s.id FOR UPDATE OF s SKIP LOCKED LIMIT 1`, now).Scan(&due.ID, &due.TaskID, &due.TaskVersionID, &due.ScheduleVersionID, &due.Expression, &due.Timezone, &storedNext)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
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
	runID, err := NewRunID()
	if err != nil {
		return "", false, err
	}
	idempotencyKey := due.ID + ":" + due.NextFireAt.UTC().Format(time.RFC3339Nano)
	if nextFire.IsZero() {
		return "", false, errors.New("schedule next fire is empty")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO runs (id, task_id, task_version_id, schedule_version_id, trigger_type, scheduled_for, state, idempotency_key) VALUES ($1, $2, $3, $4, 'SCHEDULE', $5, 'WAITING', $6) ON CONFLICT (idempotency_key) DO NOTHING`, runID, due.TaskID, due.TaskVersionID, due.ScheduleVersionID, due.NextFireAt, idempotencyKey); err != nil {
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

func normalizeScheduleDefinition(definition ScheduleDefinition) ScheduleDefinition {
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.Timezone == "" {
		definition.Timezone = "UTC"
	}
	if definition.MisfirePolicy == "" {
		definition.MisfirePolicy = "SKIP"
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
	if _, err := scheduleLocation(definition.Timezone); err != nil {
		return errors.New("schedule timezone is invalid")
	}
	return nil
}

func scheduleLocation(value string) (*time.Location, error) {
	if len(value) >= 5 && strings.HasPrefix(value, "UTC") && (value[3] == '+' || value[3] == '-') {
		parts := strings.Split(value[4:], ":")
		hours, err := strconv.Atoi(parts[0])
		if err != nil || hours > 23 || len(parts) > 2 {
			return nil, errors.New("UTC offset is invalid")
		}
		minutes := 0
		if len(parts) == 2 {
			minutes, err = strconv.Atoi(parts[1])
			if err != nil || minutes != 0 {
				return nil, errors.New("UTC offset must use whole hours")
			}
		}
		seconds := hours*60*60 + minutes*60
		if value[3] == '-' {
			seconds = -seconds
		}
		return time.FixedZone(value, seconds), nil
	}
	return time.LoadLocation(value)
}

func insertScheduleVersion(ctx context.Context, tx pgx.Tx, scheduleID, taskID string, version int, taskVersionID string, definition ScheduleDefinition) error {
	_, err := tx.Exec(ctx, `INSERT INTO schedule_versions (id, schedule_id, task_id, version, task_version_id, expression, timezone, misfire_policy, catchup_limit, start_deadline_seconds, concurrency_policy, max_concurrent_runs) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`, scheduleID+"-v"+strconv.Itoa(version), scheduleID, taskID, version, taskVersionID, definition.Expression, definition.Timezone, definition.MisfirePolicy, definition.CatchupLimit, definition.DeadlineSeconds, definition.ConcurrencyPolicy, definition.MaxConcurrentRuns)
	return err
}
