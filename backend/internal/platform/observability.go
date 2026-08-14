package platform

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
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
	OrdersPublished  atomic.Uint64
	EventsAccepted   atomic.Uint64
	SignatureRejects atomic.Uint64
}

func (m *Metrics) Snapshot() map[string]uint64 {
	return map[string]uint64{
		"orders_published":  m.OrdersPublished.Load(),
		"events_accepted":   m.EventsAccepted.Load(),
		"signature_rejects": m.SignatureRejects.Load(),
	}
}

func EnsureAuditDir(path string) error { return os.MkdirAll(filepath.Dir(path), 0700) }
