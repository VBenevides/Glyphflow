package controlplane

import (
	"context"
	"testing"
	"time"
)

func TestScheduleAndBackoff(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 34, 0, 0, time.UTC)
	next, err := (Schedule{Cron: "35 12 * * *", Timezone: "UTC"}).Next(now)
	if err != nil || next.Minute() != 35 {
		t.Fatalf("unexpected next run: %v %v", next, err)
	}
	if Backoff(4, time.Second, 5*time.Second) != 5*time.Second {
		t.Fatal("backoff did not cap")
	}
	if err := New(func(ctx context.Context) error { return nil }).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}
