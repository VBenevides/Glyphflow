package protocol

import "testing"

func TestIdentityValidation(t *testing.T) {
	order := OrderPayload{RunnerID: "worker-1", RunID: "run-1", Attempt: 2, LeaseToken: "lease-2"}
	if err := order.ValidateIdentity("worker-1", "run-1", 2, "lease-2"); err != nil {
		t.Fatal(err)
	}
	if err := order.ValidateIdentity("worker-2", "run-1", 2, "lease-2"); err == nil {
		t.Fatal("wrong runner was accepted")
	}
	event := EventPayload{RunnerID: "worker-1", RunID: "run-1", Attempt: 2, LeaseToken: "lease-2"}
	if err := event.ValidateIdentity("worker-1", "run-1", 2, "lease-2"); err != nil {
		t.Fatal(err)
	}
}
