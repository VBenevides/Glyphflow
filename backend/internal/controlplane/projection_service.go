package controlplane

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type ProjectionService struct {
	repository store.ScheduleProjectionRepository
	logger     *platform.Logger
	refreshMu  sync.Mutex
	snapshotMu sync.RWMutex
	snapshot   ProjectionReport
	available  bool
}

func NewProjectionService(repository store.ScheduleProjectionRepository, logger *platform.Logger) *ProjectionService {
	return &ProjectionService{repository: repository, logger: logger}
}

func (s *ProjectionService) Refresh(ctx context.Context) error {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	inputs, err := s.repository.ListScheduleProjection(ctx)
	if err != nil {
		s.refreshFailed(err)
		return err
	}
	report, err := BuildScheduleProjection(inputs, time.Now().UTC())
	if err != nil {
		s.refreshFailed(err)
		return err
	}
	s.snapshotMu.Lock()
	s.snapshot = report
	s.available = true
	s.snapshotMu.Unlock()
	s.event("schedule_projection.calculated", map[string]string{
		"calculated_at":     report.CalculatedAt.Format(time.RFC3339Nano),
		"window_start":      report.WindowStart.Format(time.RFC3339Nano),
		"window_end":        report.WindowEnd.Format(time.RFC3339Nano),
		"schedule_count":    strconv.Itoa(len(inputs)),
		"segment_count":     strconv.Itoa(len(report.Segments)),
		"conflict_count":    strconv.Itoa(len(report.Conflicts)),
		"freshness_seconds": strconv.FormatInt(max(0, int64(time.Since(report.CalculatedAt).Seconds())), 10),
	})
	return nil
}

func (s *ProjectionService) Snapshot() ProjectionReport {
	s.snapshotMu.RLock()
	defer s.snapshotMu.RUnlock()
	if !s.available {
		return ProjectionReport{}
	}
	return s.snapshot
}

func (s *ProjectionService) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Minute
	}
	_ = s.Refresh(ctx)
	if ctx.Err() != nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = s.Refresh(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *ProjectionService) refreshFailed(err error) {
	fields := map[string]string{"error": err.Error()}
	s.snapshotMu.RLock()
	if s.available {
		fields["last_success_calculated_at"] = s.snapshot.CalculatedAt.Format(time.RFC3339Nano)
	}
	s.snapshotMu.RUnlock()
	s.event("schedule_projection.calculation_failed", fields)
}

func (s *ProjectionService) event(name string, fields map[string]string) {
	if s.logger != nil {
		_ = s.logger.Event(name, fields)
	}
}
