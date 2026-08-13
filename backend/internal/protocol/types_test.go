package protocol

import "testing"

func TestOrderTypes(t *testing.T) {
	for _, kind := range []OrderType{OrderExecute, OrderCancel} {
		if !kind.Valid() {
			t.Fatalf("expected order type %q to be valid", kind)
		}
	}
	if OrderType("pause").Valid() {
		t.Fatal("unsupported order type was accepted")
	}
}

func TestEventTypes(t *testing.T) {
	for _, kind := range []EventType{
		EventAccepted, EventRejected, EventStarted, EventHeartbeat,
		EventCompleted, EventFailed, EventTimedOut, EventCancelled,
	} {
		if !kind.Valid() {
			t.Fatalf("expected event type %q to be valid", kind)
		}
	}
	if EventType("paused").Valid() {
		t.Fatal("unsupported event type was accepted")
	}
}
