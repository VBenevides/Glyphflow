package platform

import "testing"

func TestEventTrackerDeduplicatesAndOrdersEvents(t *testing.T) {
	tracker := NewEventTracker()
	accepted, err := tracker.Accept("event-1", "attempt-1", 1)
	if err != nil || !accepted {
		t.Fatal(err)
	}
	accepted, err = tracker.Accept("event-1", "attempt-1", 1)
	if err != nil || accepted {
		t.Fatalf("duplicate event accepted: %v %v", accepted, err)
	}
	if _, err := tracker.Accept("event-2", "attempt-1", 1); err != ErrOutOfOrderEvent {
		t.Fatalf("out-of-order event returned %v", err)
	}
	if accepted, err := tracker.Accept("event-3", "attempt-1", 2); err != nil || !accepted {
		t.Fatal(err)
	}
}
