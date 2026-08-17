package controlplane

import (
	"testing"
	"time"
)

func TestSchedulePolicySeparatesDeadlineAndConcurrency(t *testing.T) {
	policy := SchedulePolicy{Misfire: MisfireRunUpToN, Concurrency: ConcurrencyAllow, MaxConcurrentRuns: 2, StartDeadline: time.Minute, ExecutionTimeout: time.Hour}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	count, err := policy.OccurrencesDue(time.Unix(100, 0), time.Unix(70, 0), 10*time.Second, 0)
	if err != nil || count != 2 {
		t.Fatalf("unexpected catch-up count: %d %v", count, err)
	}
	policy.Concurrency = ConcurrencySkip
	policy.MaxConcurrentRuns = 0
	count, err = policy.OccurrencesDue(time.Unix(100, 0), time.Unix(70, 0), 10*time.Second, 1)
	if err != nil || count != 0 {
		t.Fatalf("skip policy started an overlapping run: %d %v", count, err)
	}
}
