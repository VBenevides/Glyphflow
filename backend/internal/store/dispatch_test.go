package store

import "testing"

func TestDispatchCreatesOneAttemptAndRetriesOutbox(t *testing.T) {
	d := NewDispatchCoordinator()
	in := DispatchAttempt{RunID: "run", AttemptID: "attempt", RunnerID: "runner", SessionID: "session", LeaseToken: "lease"}
	a, err := d.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Create(in)
	if err != nil || a.MessageID != b.MessageID {
		t.Fatalf("duplicate dispatch created another message: %#v %#v %v", a, b, err)
	}
	if len(d.Pending()) != 1 {
		t.Fatal("pending outbox missing")
	}
	if !d.MarkPublished(a.MessageID) || len(d.Pending()) != 0 {
		t.Fatal("publication state was not recorded")
	}
}
