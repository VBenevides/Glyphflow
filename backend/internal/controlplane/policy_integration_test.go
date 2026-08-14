package controlplane

import (
	"testing"
	"time"
)

func TestSchedulePoliciesAcrossReplicaClaims(t *testing.T) {
	now, next := time.Unix(100, 0), time.Unix(70, 0)
	for _, tc := range []struct {
		name    string
		misfire MisfirePolicy
		want    int
	}{
		{"skip", MisfireSkipAll, 1}, {"latest", MisfireRunLatest, 1}, {"all", MisfireRunAll, 4}, {"bounded", MisfireRunUpToN, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := SchedulePolicy{Misfire: tc.misfire, Concurrency: ConcurrencyAllow, ExecutionTimeout: time.Minute, MaxConcurrentRuns: 2}
			got, err := p.OccurrencesDue(now, next, 10*time.Second, 0)
			if err != nil || got != tc.want {
				t.Fatalf("got %d, %v", got, err)
			}
		})
	}
	p := SchedulePolicy{Misfire: MisfireRunAll, Concurrency: ConcurrencyReplace, ExecutionTimeout: time.Minute}
	if got, _ := p.OccurrencesDue(now, next, time.Minute, 1); got != 1 {
		t.Fatalf("replace policy returned %d", got)
	}
}
