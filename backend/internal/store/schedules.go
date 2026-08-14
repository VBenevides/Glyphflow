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
	ID, Name, TaskID, ScheduleType, Expression, Timezone string
	Enabled                                              bool
	MisfirePolicy, ConcurrencyPolicy                     string
	CatchupLimit, DeadlineSeconds, MaxConcurrentRuns     int
}

type ScheduleRecord struct {
	ID, Name, TaskID, ScheduleType, Expression, Timezone string
	Enabled                                              bool
	NextFireAt                                           *time.Time
	State, MisfirePolicy, ConcurrencyPolicy              string
	CatchupLimit, DeadlineSeconds, MaxConcurrentRuns     int
	ActiveVersion                                        int
}

type ScheduleRepository interface {
	List(context.Context) ([]ScheduleRecord, error)
	Find(context.Context, string) (ScheduleRecord, bool, error)
	Create(context.Context, ScheduleDefinition) (ScheduleRecord, error)
	Update(context.Context, string, ScheduleDefinition) (ScheduleRecord, error)
	Delete(context.Context, string) (bool, error)
}

type ScheduleStore struct{ pool *pgxpool.Pool }

func NewScheduleRepository(pool *pgxpool.Pool) *ScheduleStore { return &ScheduleStore{pool: pool} }

const scheduleQuery = `SELECT s.id, s.name, s.task_id, s.enabled, s.next_fire_at, sv.version, sv.schedule_type, sv.expression, sv.timezone, sv.misfire_policy, sv.catchup_limit, sv.start_deadline_seconds, sv.concurrency_policy, sv.max_concurrent_runs FROM schedules s JOIN schedule_versions sv ON sv.id = s.current_version_id`

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
	if err := row.Scan(&item.ID, &item.Name, &item.TaskID, &item.Enabled, &item.NextFireAt, &item.ActiveVersion, &item.ScheduleType, &item.Expression, &item.Timezone, &item.MisfirePolicy, &item.CatchupLimit, &item.DeadlineSeconds, &item.ConcurrencyPolicy, &item.MaxConcurrentRuns); err != nil {
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
	if _, err := tx.Exec(ctx, `INSERT INTO schedules (id, task_id, name, enabled) VALUES ($1, $2, $3, $4)`, definition.ID, definition.TaskID, definition.Name, definition.Enabled); err != nil {
		return ScheduleRecord{}, err
	}
	if err := insertScheduleVersion(ctx, tx, definition.ID, definition.TaskID, 1, taskVersionID, definition); err != nil {
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
	if _, err := tx.Exec(ctx, `UPDATE schedules SET task_id = $2, name = $3, enabled = $4, current_version_id = $5, updated_at = now() WHERE id = $1`, id, definition.TaskID, definition.Name, definition.Enabled, versionID); err != nil {
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

func normalizeScheduleDefinition(definition ScheduleDefinition) ScheduleDefinition {
	definition.Name = strings.TrimSpace(definition.Name)
	definition.ScheduleType = strings.ToLower(strings.TrimSpace(definition.ScheduleType))
	if definition.ScheduleType == "" {
		definition.ScheduleType = "cron"
	}
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
	if definition.ScheduleType != "cron" && definition.ScheduleType != "interval" {
		return errors.New("schedule type is invalid")
	}
	if _, err := time.LoadLocation(definition.Timezone); err != nil {
		return errors.New("schedule timezone is invalid")
	}
	if definition.ScheduleType == "interval" {
		interval, err := time.ParseDuration(definition.Expression)
		if err != nil || interval <= 0 {
			return errors.New("schedule interval is invalid")
		}
	}
	return nil
}

func insertScheduleVersion(ctx context.Context, tx pgx.Tx, scheduleID, taskID string, version int, taskVersionID string, definition ScheduleDefinition) error {
	_, err := tx.Exec(ctx, `INSERT INTO schedule_versions (id, schedule_id, task_id, version, task_version_id, schedule_type, expression, timezone, misfire_policy, catchup_limit, start_deadline_seconds, concurrency_policy, max_concurrent_runs) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`, scheduleID+"-v"+strconv.Itoa(version), scheduleID, taskID, version, taskVersionID, definition.ScheduleType, definition.Expression, definition.Timezone, definition.MisfirePolicy, definition.CatchupLimit, definition.DeadlineSeconds, definition.ConcurrencyPolicy, definition.MaxConcurrentRuns)
	return err
}
