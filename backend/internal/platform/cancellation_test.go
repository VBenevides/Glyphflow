package platform

import "testing"

func TestCancellationIsAttemptSpecific(t *testing.T) {
	active := Cancellation{RunID: "run", AttemptID: "attempt-2", SessionID: "session", LeaseToken: "lease", Fencing: 2}
	if err := ValidateCancellation(active, active); err != nil {
		t.Fatal(err)
	}
	old := active
	old.AttemptID = "attempt-1"
	if err := ValidateCancellation(old, active); err == nil {
		t.Fatal("old cancellation affected new attempt")
	}
}

func TestCancellationCompletionWinsRace(t *testing.T) {
	c := Cancellation{RunID: "r", AttemptID: "a", SessionID: "s", LeaseToken: "l", Fencing: 1}
	if state, err := ApplyCancellation(c, c, "running", true); err != nil || state != "completed" {
		t.Fatalf("completion race: %s %v", state, err)
	}
	if state, err := ApplyCancellation(c, c, "running", false); err != nil || state != "cancelling" {
		t.Fatalf("cancel transition: %s %v", state, err)
	}
}
