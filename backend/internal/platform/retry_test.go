package platform

import (
	"testing"
	"time"
)

func TestRetryPolicyAndRunAggregation(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 2, RetryableExitCodes: map[int]bool{7: true}, BaseDelay: time.Second, MaxDelay: time.Minute}
	state, delay := p.Decide(1, 7, "exit")
	if state != "retry_wait" || delay != time.Second {
		t.Fatalf("retry decision: %s %v", state, delay)
	}
	state, _ = p.Decide(2, 7, "exit")
	if state != "failed" {
		t.Fatal("attempt limit ignored")
	}
	r := new(RunAggregator)
	if r.Apply("failed", true, 2) != "retry_wait" || r.Apply("failed", true, 2) != "failed" || r.Apply("unknown", false, 2) != "unknown" {
		t.Fatalf("aggregation state: %#v", r)
	}
}
