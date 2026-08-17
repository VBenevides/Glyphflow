package protocol

import "testing"

func TestReplayGuardRejectsDuplicateIdentifiers(t *testing.T) {
	guard := NewReplayGuard()
	if err := guard.Accept("event-1"); err != nil {
		t.Fatal(err)
	}
	if err := guard.Accept("event-1"); err == nil {
		t.Fatal("replayed event was accepted")
	}
}
