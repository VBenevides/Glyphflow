package api

import (
	"context"
	"net/http"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type SystemMetricsService struct {
	Metrics    *platform.Metrics
	Ready      func(context.Context) error
	Signals    func(context.Context) (platform.OperationalSignals, error)
	Storage    func(context.Context) (platform.StoragePressure, error)
	Thresholds platform.AlertThresholds
	Tracker    *platform.AlertTracker
	Logger     *platform.Logger
}

func NewSystemMetricsService(metrics *platform.Metrics, ready func(context.Context) error, logger *platform.Logger) *SystemMetricsService {
	return &SystemMetricsService{
		Metrics:    metrics,
		Ready:      ready,
		Thresholds: platform.DefaultAlertThresholds(),
		Tracker:    platform.NewAlertTracker(),
		Logger:     logger,
	}
}

func (s *SystemMetricsService) snapshot(ctx context.Context) (platform.SystemMetricsResponse, error) {
	ready := true
	if s.Ready != nil {
		ready = s.Ready(ctx) == nil
	}
	metrics := s.Metrics
	if metrics == nil {
		metrics = new(platform.Metrics)
	}
	signals := platform.OperationalSignals{}
	if s.Signals != nil {
		var err error
		signals, err = s.Signals(ctx)
		if err != nil {
			return platform.SystemMetricsResponse{}, err
		}
	}
	if s.Storage != nil {
		storage, err := s.Storage(ctx)
		if err != nil {
			return platform.SystemMetricsResponse{}, err
		}
		signals.Disk.FreeBytes = storage.FreeBytes
		signals.Disk.FreePercent = storage.FreePercent
		signals.Disk.State = storage.State
		signals.Disk.Code = storage.Code
	}
	alerts := platform.EvaluateOperationalAlerts(signals, s.Thresholds)
	if s.Tracker != nil && s.Logger != nil {
		_ = s.Tracker.Emit(s.Logger, alerts)
	}
	return platform.SystemMetricsResponse{
		GeneratedAt: time.Now().UTC(),
		Ready:       ready,
		Metrics:     metrics.Snapshot(),
		Signals:     signals,
		Alerts:      alerts,
	}, nil
}

func (s *SystemMetricsService) Evaluate(ctx context.Context) error {
	_, err := s.snapshot(ctx)
	return err
}

func (s *SystemMetricsService) metrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	response, err := s.snapshot(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "metrics unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, response)
}
