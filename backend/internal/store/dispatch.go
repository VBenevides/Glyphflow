package store

import (
	"errors"
	"sync"
)

type DispatchAttempt struct {
	RunID, AttemptID, RunnerID, SessionID, LeaseToken string
}

type DispatchOutbox struct {
	MessageID, AttemptID string
	Published            bool
}

type DispatchCoordinator struct {
	mu       sync.Mutex
	attempts map[string]DispatchAttempt
	outbox   map[string]DispatchOutbox
}

func NewDispatchCoordinator() *DispatchCoordinator {
	return &DispatchCoordinator{attempts: map[string]DispatchAttempt{}, outbox: map[string]DispatchOutbox{}}
}

func (d *DispatchCoordinator) Create(attempt DispatchAttempt) (DispatchOutbox, error) {
	if attempt.RunID == "" || attempt.AttemptID == "" || attempt.RunnerID == "" || attempt.SessionID == "" || attempt.LeaseToken == "" {
		return DispatchOutbox{}, errors.New("dispatch fields are required")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if existing, ok := d.attempts[attempt.RunID]; ok {
		return d.outbox[existing.AttemptID], nil
	}
	out := DispatchOutbox{MessageID: attempt.AttemptID, AttemptID: attempt.AttemptID}
	d.attempts[attempt.RunID], d.outbox[attempt.AttemptID] = attempt, out
	return out, nil
}

func (d *DispatchCoordinator) MarkPublished(messageID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	out, ok := d.outbox[messageID]
	if !ok {
		return false
	}
	out.Published = true
	d.outbox[messageID] = out
	return true
}

func (d *DispatchCoordinator) Pending() []DispatchOutbox {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []DispatchOutbox
	for _, item := range d.outbox {
		if !item.Published {
			out = append(out, item)
		}
	}
	return out
}
