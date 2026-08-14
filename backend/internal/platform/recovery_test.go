package platform

import "testing"

func TestAmbiguousExecutionUsesExplicitPolicy(t *testing.T) {
	for policy, expected := range map[AmbiguityPolicy]string{RetryAmbiguous: "retry_wait", ManualAmbiguous: "unknown", FailedAmbiguous: "failed"} {
		state, err := ResolveAmbiguous(policy)
		if err != nil || state != expected {
			t.Fatalf("policy %s returned %s %v", policy, state, err)
		}
	}
	if _, err := ResolveAmbiguous("invalid"); err == nil {
		t.Fatal("invalid ambiguity policy accepted")
	}
}

func TestStateMachineAllowsLegalTransitionsAndIncrementsVersion(t *testing.T) {
	machine, err := NewStateMachine("waiting")
	if err != nil {
		t.Fatal(err)
	}

	state, version := machine.Snapshot()
	if state != "waiting" || version != 0 {
		t.Fatalf("initial snapshot = %s/%d", state, version)
	}
	if err := machine.CompareAndSwap("waiting", 0, "running"); err != nil {
		t.Fatal(err)
	}
	if err := machine.CompareAndSwap("running", 1, "succeeded"); err != nil {
		t.Fatal(err)
	}
	state, version = machine.Snapshot()
	if state != "succeeded" || version != 2 {
		t.Fatalf("final snapshot = %s/%d", state, version)
	}
}

func TestStateMachineRejectsTerminalRegression(t *testing.T) {
	machine, err := NewStateMachine("succeeded")
	if err != nil {
		t.Fatal(err)
	}
	if err := machine.CompareAndSwap("succeeded", 0, "running"); err == nil {
		t.Fatal("terminal state regressed")
	}
	state, version := machine.Snapshot()
	if state != "succeeded" || version != 0 {
		t.Fatalf("rejected transition changed snapshot to %s/%d", state, version)
	}
}

func TestStateMachineRejectsExpectedStateAndVersionConflicts(t *testing.T) {
	machine, err := NewStateMachine("waiting")
	if err != nil {
		t.Fatal(err)
	}
	if err := machine.CompareAndSwap("waiting", 1, "running"); err == nil {
		t.Fatal("stale version accepted")
	}
	if err := machine.CompareAndSwap("stale", 0, "running"); err == nil {
		t.Fatal("stale state accepted")
	}
}
