package platform

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Logger struct {
	Out io.Writer
	mu  sync.Mutex
}

func (l *Logger) Event(name string, fields map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	payload := map[string]any{"time": time.Now().UTC(), "event": name, "fields": Redact(fields)}
	return json.NewEncoder(l.Out).Encode(payload)
}

type AuditRecord struct {
	Actor   string    `json:"actor"`
	Action  string    `json:"action"`
	Target  string    `json:"target"`
	Address string    `json:"address"`
	Result  string    `json:"result"`
	At      time.Time `json:"at"`
}

type SecurityAuditRecord struct {
	ActorType  string            `json:"actor_type"`
	ActorID    string            `json:"actor_id"`
	SessionID  string            `json:"session_id"`
	Endpoint   string            `json:"endpoint"`
	TargetType string            `json:"target_type"`
	TargetID   string            `json:"target_id"`
	Result     string            `json:"result"`
	Before     map[string]string `json:"before,omitempty"`
	After      map[string]string `json:"after,omitempty"`
	At         time.Time         `json:"at"`
}
type AuditLog struct {
	mu      sync.Mutex
	Records []AuditRecord
	path    string
}

func OpenAuditLog(path string) (*AuditLog, error) {
	log := &AuditLog{path: path}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return log, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &log.Records); err != nil {
			return nil, err
		}
	}
	return log, nil
}

func (a *AuditLog) Add(record AuditRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Records = append(a.Records, record)
	if a.path == "" {
		return
	}
	raw, err := json.Marshal(a.Records)
	if err != nil {
		return
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err == nil {
		_ = os.Rename(tmp, a.path)
	}
}

func (a *AuditLog) AddSecurity(record SecurityAuditRecord) {
	a.Add(AuditRecord{
		Actor:  record.ActorType + ":" + record.ActorID,
		Action: record.Endpoint,
		Target: record.TargetType + ":" + record.TargetID,
		Result: record.Result,
		At:     record.At,
	})
}

type Metrics struct {
	OrdersPublished   atomic.Uint64
	EventsAccepted    atomic.Uint64
	AuditAppendErrors atomic.Uint64
	SignatureRejects  atomic.Uint64
	SchedulerLag      atomic.Uint64
	DispatchLag       atomic.Uint64
	ActiveLeases      atomic.Uint64
	UnknownAttempts   atomic.Uint64
	InboxDuplicates   atomic.Uint64
	OutboxRetries     atomic.Uint64
	LoginFailures     atomic.Uint64
	RefreshReplays    atomic.Uint64
	PermissionDenials atomic.Uint64
}

func (m *Metrics) Snapshot() map[string]uint64 {
	return map[string]uint64{
		"orders_published":    m.OrdersPublished.Load(),
		"events_accepted":     m.EventsAccepted.Load(),
		"audit_append_errors": m.AuditAppendErrors.Load(),
		"signature_rejects":   m.SignatureRejects.Load(),
		"scheduler_lag":       m.SchedulerLag.Load(), "dispatch_lag": m.DispatchLag.Load(), "active_leases": m.ActiveLeases.Load(), "unknown_attempts": m.UnknownAttempts.Load(),
		"inbox_duplicates": m.InboxDuplicates.Load(), "outbox_retries": m.OutboxRetries.Load(), "login_failures": m.LoginFailures.Load(), "refresh_replays": m.RefreshReplays.Load(), "permission_denials": m.PermissionDenials.Load(),
	}
}

type DiskSignals struct {
	FreeBytes   uint64  `json:"freeBytes"`
	FreePercent float64 `json:"freePercent"`
}

type DeadLetterSignals struct {
	Open             uint64 `json:"open"`
	OldestAgeSeconds uint64 `json:"oldestAgeSeconds"`
}

type OperationalSignals struct {
	QueueLagSeconds uint64            `json:"queueLagSeconds"`
	DeadLetters     DeadLetterSignals `json:"deadLetters"`
	StuckRuns       uint64            `json:"stuckRuns"`
	Disk            DiskSignals       `json:"disk"`
}

type OperationalAlert struct {
	Code      string  `json:"code"`
	Severity  string  `json:"severity"`
	Status    string  `json:"status"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
}

type SystemMetricsResponse struct {
	GeneratedAt time.Time          `json:"generatedAt"`
	Ready       bool               `json:"ready"`
	Metrics     map[string]uint64  `json:"metrics"`
	Signals     OperationalSignals `json:"signals"`
	Alerts      []OperationalAlert `json:"alerts"`
}

type AlertThresholds struct {
	QueueLagWarningSeconds       uint64
	QueueLagCriticalSeconds      uint64
	DeadLettersWarning           uint64
	DeadLettersCritical          uint64
	DeadLetterAgeWarningSeconds  uint64
	DeadLetterAgeCriticalSeconds uint64
	DiskFreeWarningPercent       float64
	DiskFreeCriticalPercent      float64
	StuckRunsWarning             uint64
	StuckRunsCritical            uint64
}

func DefaultAlertThresholds() AlertThresholds {
	return AlertThresholds{
		QueueLagWarningSeconds:       30,
		QueueLagCriticalSeconds:      300,
		DeadLettersWarning:           1,
		DeadLettersCritical:          10,
		DeadLetterAgeWarningSeconds:  300,
		DeadLetterAgeCriticalSeconds: 1800,
		DiskFreeWarningPercent:       20,
		DiskFreeCriticalPercent:      10,
		StuckRunsWarning:             1,
		StuckRunsCritical:            10,
	}
}

func EvaluateOperationalAlerts(signals OperationalSignals, thresholds AlertThresholds) []OperationalAlert {
	alerts := make([]OperationalAlert, 0, 5)
	appendHighAlert := func(code string, value, warning, critical float64) {
		if critical > 0 && value >= critical {
			alerts = append(alerts, OperationalAlert{Code: code, Severity: "critical", Status: "firing", Value: value, Threshold: critical})
		} else if warning > 0 && value >= warning {
			alerts = append(alerts, OperationalAlert{Code: code, Severity: "warning", Status: "firing", Value: value, Threshold: warning})
		}
	}
	appendLowAlert := func(code string, value, warning, critical float64) {
		if critical > 0 && value <= critical {
			alerts = append(alerts, OperationalAlert{Code: code, Severity: "critical", Status: "firing", Value: value, Threshold: critical})
		} else if warning > 0 && value <= warning {
			alerts = append(alerts, OperationalAlert{Code: code, Severity: "warning", Status: "firing", Value: value, Threshold: warning})
		}
	}
	appendHighAlert("queue_lag", float64(signals.QueueLagSeconds), float64(thresholds.QueueLagWarningSeconds), float64(thresholds.QueueLagCriticalSeconds))
	appendHighAlert("dead_letters_open", float64(signals.DeadLetters.Open), float64(thresholds.DeadLettersWarning), float64(thresholds.DeadLettersCritical))
	appendHighAlert("dead_letters_oldest_age", float64(signals.DeadLetters.OldestAgeSeconds), float64(thresholds.DeadLetterAgeWarningSeconds), float64(thresholds.DeadLetterAgeCriticalSeconds))
	appendLowAlert("disk_free_percent", signals.Disk.FreePercent, thresholds.DiskFreeWarningPercent, thresholds.DiskFreeCriticalPercent)
	appendHighAlert("stuck_runs", float64(signals.StuckRuns), float64(thresholds.StuckRunsWarning), float64(thresholds.StuckRunsCritical))
	return alerts
}

type AlertTracker struct {
	mu     sync.Mutex
	states map[string]OperationalAlert
}

func NewAlertTracker() *AlertTracker {
	return &AlertTracker{states: make(map[string]OperationalAlert)}
}

func (t *AlertTracker) Emit(logger *Logger, current []OperationalAlert) error {
	if t == nil {
		return errors.New("alert tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if logger == nil {
		return errors.New("alert logger is required")
	}
	next := make(map[string]OperationalAlert, len(current))
	transitions := make([]OperationalAlert, 0, len(current)+len(t.states))
	for _, alert := range current {
		previous, exists := t.states[alert.Code]
		next[alert.Code] = alert
		if !exists || previous.Severity != alert.Severity || previous.Status != alert.Status || previous.Threshold != alert.Threshold {
			transitions = append(transitions, alert)
		}
	}
	for code, previous := range t.states {
		if _, exists := next[code]; !exists {
			previous.Status = "resolved"
			transitions = append(transitions, previous)
		}
	}
	for _, alert := range transitions {
		if err := logger.Event("system.alert", map[string]string{
			"code":      alert.Code,
			"severity":  alert.Severity,
			"status":    alert.Status,
			"value":     strconv.FormatFloat(alert.Value, 'f', -1, 64),
			"threshold": strconv.FormatFloat(alert.Threshold, 'f', -1, 64),
		}); err != nil {
			return err
		}
	}
	t.states = next
	return nil
}
