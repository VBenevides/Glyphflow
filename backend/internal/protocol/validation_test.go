package protocol

import "testing"

func TestOrderExecutionValidationRejectsUnsafeReferences(t *testing.T) {
	order := OrderPayload{OrderID: "order", RunID: "run", RunnerID: "worker", LeaseToken: "lease", WorkingDir: "/srv", TimeoutSeconds: 1, Command: []string{"echo"}, SecretRefs: []string{"../password"}}
	if err := order.ValidateExecution(); err == nil {
		t.Fatal("unsafe secret reference was accepted")
	}
}
