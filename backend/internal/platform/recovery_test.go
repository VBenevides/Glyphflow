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
