package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultLogMonthsKeep           = 3
	DefaultAuditMonthsKeep         = 12
	DefaultRunnerMetricsMonthsKeep = 3
	defaultRetentionBatch          = 100
	maxRetentionBatch              = 1000
)

type RetentionPolicy struct {
	LogMonthsKeep           int
	AuditMonthsKeep         int
	RunnerMetricsMonthsKeep int
}

func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{LogMonthsKeep: DefaultLogMonthsKeep, AuditMonthsKeep: DefaultAuditMonthsKeep, RunnerMetricsMonthsKeep: DefaultRunnerMetricsMonthsKeep}
}

func (p RetentionPolicy) valid() bool {
	return p.LogMonthsKeep > 0 && p.AuditMonthsKeep > 0 && p.RunnerMetricsMonthsKeep > 0
}

type RetentionResult struct {
	Runs, DeadLetters, AuditEvents, RunnerMetrics int64
}

type RetentionStore struct{ pool *pgxpool.Pool }

func NewRetentionRepository(pool *pgxpool.Pool) *RetentionStore { return &RetentionStore{pool: pool} }

func (s *RetentionStore) SetLegalHold(ctx context.Context, dataClass, dataID string, held bool, reason string) error {
	if s == nil || s.pool == nil {
		return errors.New("retention storage is unavailable")
	}
	if dataClass != "run" && dataClass != "dead_letter" && dataClass != "audit" || dataID == "" {
		return errors.New("retention legal hold target is invalid")
	}
	if held {
		if reason == "" {
			return errors.New("retention legal hold reason is required")
		}
		_, err := s.pool.Exec(ctx, `INSERT INTO retention_legal_holds (data_class, data_id, reason) VALUES ($1, $2, $3) ON CONFLICT (data_class, data_id) DO UPDATE SET reason = EXCLUDED.reason`, dataClass, dataID, reason)
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM retention_legal_holds WHERE data_class = $1 AND data_id = $2`, dataClass, dataID)
	return err
}

func (s *RetentionStore) Purge(ctx context.Context, now time.Time, policy RetentionPolicy, batch int) (RetentionResult, error) {
	if s == nil || s.pool == nil {
		return RetentionResult{}, errors.New("retention storage is unavailable")
	}
	if !policy.valid() {
		return RetentionResult{}, errors.New("retention policy is invalid")
	}
	batch = boundedRetentionBatch(batch)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RetentionResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('glyphflow.retention_cleanup', 'on', true)`); err != nil {
		return RetentionResult{}, err
	}
	result, err := purgeRows(ctx, tx, now.AddDate(0, -policy.LogMonthsKeep, 0), now.AddDate(0, -policy.AuditMonthsKeep, 0), now.AddDate(0, -policy.RunnerMetricsMonthsKeep, 0), batch, false)
	if err != nil {
		return RetentionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RetentionResult{}, err
	}
	return result, nil
}

func (s *RetentionStore) PurgeCriticalRuns(ctx context.Context, now time.Time, freePercent func() (float64, error), batch int) (RetentionResult, error) {
	if s == nil || s.pool == nil || freePercent == nil {
		return RetentionResult{}, errors.New("critical retention cleanup is unavailable")
	}
	batch = boundedRetentionBatch(batch)
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var total RetentionResult
	for range 100 {
		free, err := freePercent()
		if err != nil {
			return total, err
		}
		if free >= 15 {
			return total, nil
		}
		result, err := s.purgeRunBatch(ctx, now.Add(-7*24*time.Hour), batch)
		if err != nil {
			return total, err
		}
		total.Runs += result.Runs
		if result.Runs == 0 {
			return total, nil
		}
	}
	return total, nil
}

func boundedRetentionBatch(batch int) int {
	if batch < 1 || batch > maxRetentionBatch {
		return defaultRetentionBatch
	}
	return batch
}

func purgeRows(ctx context.Context, tx pgx.Tx, runCutoff, auditCutoff, metricsCutoff time.Time, batch int, criticalOnly bool) (RetentionResult, error) {
	result := RetentionResult{}
	if criticalOnly {
		deleted, err := deleteRunBatch(ctx, tx, runCutoff, batch)
		result.Runs = deleted
		return result, err
	}
	deleted, err := deleteRunBatch(ctx, tx, runCutoff, batch)
	if err != nil {
		return result, err
	}
	result.Runs = deleted
	if result.DeadLetters, err = deleteDeadLetterBatch(ctx, tx, runCutoff, batch); err != nil {
		return result, err
	}
	result.AuditEvents, err = deleteAuditBatch(ctx, tx, auditCutoff, batch)
	if err != nil {
		return result, err
	}
	result.RunnerMetrics, err = deleteRunnerMetricsBatch(ctx, tx, metricsCutoff, batch)
	return result, err
}

func deleteRunnerMetricsBatch(ctx context.Context, tx pgx.Tx, cutoff time.Time, batch int) (int64, error) {
	result, err := tx.Exec(ctx, `WITH candidates AS (SELECT runner_id, sampled_at FROM runner_metrics WHERE sampled_at < $1 ORDER BY sampled_at, runner_id LIMIT $2) DELETE FROM runner_metrics m USING candidates c WHERE m.runner_id = c.runner_id AND m.sampled_at = c.sampled_at`, cutoff, batch)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func deleteRunBatch(ctx context.Context, tx pgx.Tx, cutoff time.Time, batch int) (int64, error) {
	result, err := tx.Exec(ctx, `WITH candidates AS (SELECT r.id FROM runs r WHERE r.state IN ('SUCCEEDED', 'FAILED', 'CANCELLED') AND r.completed_at IS NOT NULL AND r.completed_at < $1 AND NOT EXISTS (SELECT 1 FROM retention_legal_holds h WHERE h.data_class = 'run' AND h.data_id = r.id) ORDER BY r.completed_at, r.id FOR UPDATE SKIP LOCKED LIMIT $2) DELETE FROM runs r USING candidates c WHERE r.id = c.id`, cutoff, batch)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func deleteDeadLetterBatch(ctx context.Context, tx pgx.Tx, cutoff time.Time, batch int) (int64, error) {
	result, err := tx.Exec(ctx, `WITH candidates AS (SELECT d.id FROM dead_letters d WHERE d.state IN ('RECONCILED', 'DISCARDED') AND d.last_failed_at < $1 AND NOT EXISTS (SELECT 1 FROM retention_legal_holds h WHERE h.data_class = 'dead_letter' AND h.data_id = d.id) ORDER BY d.last_failed_at, d.id FOR UPDATE SKIP LOCKED LIMIT $2) DELETE FROM dead_letters d USING candidates c WHERE d.id = c.id`, cutoff, batch)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func deleteAuditBatch(ctx context.Context, tx pgx.Tx, cutoff time.Time, batch int) (int64, error) {
	result, err := tx.Exec(ctx, `WITH candidates AS (SELECT a.id FROM audit_events a WHERE a.created_at < $1 AND NOT EXISTS (SELECT 1 FROM retention_legal_holds h WHERE h.data_class = 'audit' AND h.data_id = a.id) ORDER BY a.created_at, a.id LIMIT $2) DELETE FROM audit_events a USING candidates c WHERE a.id = c.id`, cutoff, batch)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (s *RetentionStore) purgeRunBatch(ctx context.Context, cutoff time.Time, batch int) (RetentionResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RetentionResult{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('glyphflow.retention_cleanup', 'on', true)`); err != nil {
		return RetentionResult{}, err
	}
	result, err := purgeRows(ctx, tx, cutoff, time.Time{}, time.Time{}, batch, true)
	if err != nil {
		return RetentionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RetentionResult{}, err
	}
	return result, nil
}
