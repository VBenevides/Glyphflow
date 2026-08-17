package controlplane

import (
	"testing"
	"time"
)

func TestDueScheduleClaimAdvancesExactlyOnce(t *testing.T) {
	now := time.Now()
	queue := NewDueScheduleQueue()
	queue.Add(DueSchedule{ID: "schedule-1", NextFire: now.Add(-time.Minute), Interval: time.Minute})
	first, ok := queue.Claim(now)
	if !ok || first.NextFire.Before(now.Add(-time.Second)) {
		t.Fatalf("schedule was not advanced: %#v %v", first, ok)
	}
	if _, ok := queue.Claim(now); ok {
		t.Fatal("same occurrence was claimed twice")
	}
}
