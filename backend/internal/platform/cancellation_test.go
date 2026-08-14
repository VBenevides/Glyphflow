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
