package platform

import "sync"

type EventTracker struct {
	mu       sync.Mutex
	seen     map[string]struct{}
	sequence map[string]uint64
}

func NewEventTracker() *EventTracker {
	return &EventTracker{seen: make(map[string]struct{}), sequence: make(map[string]uint64)}
}

func (t *EventTracker) Accept(eventID, attemptID string, sequence uint64) (bool, error) {
	if eventID == "" || attemptID == "" || sequence == 0 {
		return false, ErrInvalidEvent
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.seen[eventID]; ok {
		return false, nil
	}
	if previous := t.sequence[attemptID]; sequence <= previous {
		return false, ErrOutOfOrderEvent
	}
	t.seen[eventID] = struct{}{}
	t.sequence[attemptID] = sequence
	return true, nil
}

var ErrInvalidEvent = eventError("invalid event")
var ErrOutOfOrderEvent = eventError("out-of-order event")

type eventError string

func (e eventError) Error() string { return string(e) }
