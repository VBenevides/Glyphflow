package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestSystemMetricsRouteIsProtectedAndReturnsContract(t *testing.T) {
	metrics := new(platform.Metrics)
	metrics.SignatureRejects.Add(2)
	granted := false
	server := Server{
		Auth: func(*http.Request) (Claims, bool) { return Claims{}, true },
		Permissions: func(Claims) map[string]bool {
			if granted {
				return map[string]bool{"system.metrics.read": true}
			}
			return nil
		},
		SystemMetrics: &SystemMetricsService{
			Metrics: metrics,
			Signals: func(context.Context) (platform.OperationalSignals, error) {
				return platform.OperationalSignals{QueueLagSeconds: 4, Disk: platform.DiskSignals{FreePercent: 80}}, nil
			},
			Thresholds: platform.DefaultAlertThresholds(),
			Tracker:    platform.NewAlertTracker(),
			Logger:     &platform.Logger{Out: &bytes.Buffer{}},
		},
	}
	handler := server.Handler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/metrics", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("forbidden status = %d", response.Code)
	}
	granted = true
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("allowed status = %d, body = %s", response.Code, response.Body.String())
	}
	var body platform.SystemMetricsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Metrics["signature_rejects"] != 2 || body.Signals.QueueLagSeconds != 4 || !body.Ready {
		t.Fatalf("unexpected system metrics: %+v", body)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/metrics", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status = %d", response.Code)
	}
}

func TestSystemMetricsRouteReportsSignalProviderFailure(t *testing.T) {
	server := Server{
		Auth:        func(*http.Request) (Claims, bool) { return Claims{}, true },
		Permissions: func(Claims) map[string]bool { return map[string]bool{"system.metrics.read": true} },
		SystemMetrics: &SystemMetricsService{
			Signals: func(context.Context) (platform.OperationalSignals, error) {
				return platform.OperationalSignals{}, errors.New("database unavailable")
			},
		},
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/metrics", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("failure status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSystemMetricsEvaluatesAlertsWithoutHTTP(t *testing.T) {
	var log bytes.Buffer
	service := &SystemMetricsService{
		Signals: func(context.Context) (platform.OperationalSignals, error) {
			return platform.OperationalSignals{QueueLagSeconds: 60}, nil
		},
		Thresholds: platform.DefaultAlertThresholds(),
		Tracker:    platform.NewAlertTracker(),
		Logger:     &platform.Logger{Out: &log},
	}
	if err := service.Evaluate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), `"event":"system.alert"`) {
		t.Fatalf("periodic evaluation emitted no alert: %s", log.String())
	}
}

func TestSystemMetricsUsesDatabaseStorageSignal(t *testing.T) {
	service := &SystemMetricsService{
		Storage: func(context.Context) (platform.StoragePressure, error) {
			return platform.StoragePressure{State: platform.StorageWarning, Code: "database_storage_warning", FreeBytes: 42, FreePercent: 15}, nil
		},
	}
	response, err := service.snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Signals.Disk.State != platform.StorageWarning || response.Signals.Disk.Code != "database_storage_warning" || response.Signals.Disk.FreeBytes != 42 || response.Signals.Disk.FreePercent != 15 {
		t.Fatalf("database storage signal = %#v", response.Signals.Disk)
	}
}
