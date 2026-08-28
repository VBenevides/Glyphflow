package platform

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposeOnlyFixedLowCardinalityNames(t *testing.T) {
	m := new(Metrics)
	m.LoginFailures.Add(2)
	m.PermissionDenials.Add(1)
	m.AuditAppendErrors.Add(3)
	snapshot := m.Snapshot()
	if snapshot["login_failures"] != 2 || snapshot["permission_denials"] != 1 || snapshot["audit_append_errors"] != 3 {
		t.Fatalf("metrics missing: %#v", snapshot)
	}
}

func TestSystemMetricsContractAndAlertTransitions(t *testing.T) {
	response := SystemMetricsResponse{
		GeneratedAt: time.Unix(10, 0).UTC(),
		Ready:       true,
		Metrics:     (new(Metrics)).Snapshot(),
		Signals: OperationalSignals{
			QueueLagSeconds: 60,
			DeadLetters:     DeadLetterSignals{Open: 1},
			Disk:            DiskSignals{FreeBytes: 100, FreePercent: 15},
		},
		Alerts: EvaluateOperationalAlerts(OperationalSignals{QueueLagSeconds: 60, Disk: DiskSignals{FreePercent: 15}}, DefaultAlertThresholds()),
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"generatedAt"`, `"ready"`, `"metrics"`, `"signals"`, `"alerts"`, `"queueLagSeconds"`, `"freePercent"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("response is missing %s: %s", field, raw)
		}
	}

	var log bytes.Buffer
	tracker := NewAlertTracker()
	alerts := EvaluateOperationalAlerts(response.Signals, DefaultAlertThresholds())
	if err := tracker.Emit(&Logger{Out: &log}, alerts); err != nil {
		t.Fatal(err)
	}
	firstEventCount := strings.Count(log.String(), `"event":"system.alert"`)
	if firstEventCount != 3 {
		t.Fatalf("expected three firing alerts, got %d: %s", firstEventCount, log.String())
	}
	if err := tracker.Emit(&Logger{Out: &log}, alerts); err != nil {
		t.Fatal(err)
	}
	if strings.Count(log.String(), `"event":"system.alert"`) != firstEventCount {
		t.Fatal("unchanged alerts emitted a duplicate transition")
	}
	if err := tracker.Emit(&Logger{Out: &log}, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Count(log.String(), `"status":"resolved"`) != 3 {
		t.Fatalf("expected three resolved alerts: %s", log.String())
	}
	if strings.Contains(log.String(), "runnerId") || strings.Contains(log.String(), "userId") {
		t.Fatal("alert event contains dynamic identity labels")
	}
}

func TestEvaluateOperationalAlertsUsesCriticalBeforeWarning(t *testing.T) {
	thresholds := AlertThresholds{QueueLagWarningSeconds: 10, QueueLagCriticalSeconds: 20, DiskFreeWarningPercent: 20, DiskFreeCriticalPercent: 10}
	alerts := EvaluateOperationalAlerts(OperationalSignals{QueueLagSeconds: 20, Disk: DiskSignals{FreePercent: 5}}, thresholds)
	if len(alerts) != 2 || alerts[0].Severity != "critical" || alerts[1].Severity != "critical" {
		t.Fatalf("unexpected alert severities: %#v", alerts)
	}
}

func TestEvaluateOperationalAlertsReportsUnavailableStorage(t *testing.T) {
	alerts := EvaluateOperationalAlerts(OperationalSignals{Disk: DiskSignals{State: StorageUnavailable}}, DefaultAlertThresholds())
	if len(alerts) != 1 || alerts[0].Code != "storage_unavailable" || alerts[0].Severity != "critical" {
		t.Fatalf("unavailable storage alerts = %#v", alerts)
	}
}
