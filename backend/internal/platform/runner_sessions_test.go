package platform

import "testing"

func TestRunnerSessionReplacementDeactivatesOldSession(t *testing.T) {
	registry := NewRunnerSessionRegistry()
	if _, err := registry.Connect("runner-1", "session-1"); err != nil {
		t.Fatal(err)
	}
	previous, err := registry.Connect("runner-1", "session-2")
	if err != nil || previous != "session-1" || registry.IsActive("runner-1", "session-1") || !registry.IsActive("runner-1", "session-2") {
		t.Fatalf("session replacement failed: %q %v", previous, err)
	}
}
