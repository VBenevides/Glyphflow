package store

import (
	"context"
	"errors"
	"math"
	"time"
)

type RunnerMetricsSample struct {
	CPUPercent       float64
	MemoryPercent    float64
	MemoryUsedBytes  int64
	MemoryTotalBytes int64
}

func (s RunnerMetricsSample) validate() error {
	if math.IsNaN(s.CPUPercent) || math.IsInf(s.CPUPercent, 0) || s.CPUPercent < 0 || s.CPUPercent > 100 {
		return errors.New("runner CPU percentage is invalid")
	}
	if math.IsNaN(s.MemoryPercent) || math.IsInf(s.MemoryPercent, 0) || s.MemoryPercent < 0 || s.MemoryPercent > 100 {
		return errors.New("runner memory percentage is invalid")
	}
	if s.MemoryUsedBytes < 0 || s.MemoryTotalBytes <= 0 {
		return errors.New("runner memory size is invalid")
	}
	return nil
}

type RunnerMetricsRecord struct {
	SampledAt        time.Time
	CPUPercent       float64
	MemoryPercent    float64
	MemoryUsedBytes  int64
	MemoryTotalBytes int64
}

type RunnerMetricsLister interface {
	ListRunnerMetrics(context.Context, string, time.Time, time.Time, int) ([]RunnerMetricsRecord, error)
}

func (s *RunnerStore) ListRunnerMetrics(ctx context.Context, runnerID string, from, to time.Time, limit int) ([]RunnerMetricsRecord, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("runner metrics storage is unavailable")
	}
	if runnerID == "" || from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, errors.New("runner metrics range is invalid")
	}
	if limit < 1 || limit > 2000 {
		limit = 2000
	}
	rows, err := s.pool.Query(ctx, `SELECT sampled_at, cpu_percent, memory_percent, memory_used_bytes, memory_total_bytes
		FROM runner_metrics
		WHERE runner_id = $1 AND sampled_at >= $2 AND sampled_at <= $3
		ORDER BY sampled_at ASC LIMIT $4`, runnerID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]RunnerMetricsRecord, 0)
	for rows.Next() {
		var item RunnerMetricsRecord
		if err := rows.Scan(&item.SampledAt, &item.CPUPercent, &item.MemoryPercent, &item.MemoryUsedBytes, &item.MemoryTotalBytes); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
