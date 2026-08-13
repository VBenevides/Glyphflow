package platform

import (
	"encoding/json"
	"io"
	"sync"
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
	Actor, Action, Target, Address, Result string
	At                                     time.Time
}
type AuditLog struct {
	mu      sync.Mutex
	Records []AuditRecord
}

func (a *AuditLog) Add(record AuditRecord) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Records = append(a.Records, record)
}
