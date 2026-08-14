package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestScheduleAndBackoff(t *testing.T) {
	now := time.Date(2026, 1, 2, 12, 34, 0, 0, time.UTC)
	next, err := (Schedule{Cron: "35 12 * * *", Timezone: "UTC"}).Next(now)
	if err != nil || next.Minute() != 35 {
		t.Fatalf("unexpected next run: %v %v", next, err)
	}
	if platform.RetryDelay(4, time.Second, 5*time.Second) != 5*time.Second {
		t.Fatal("backoff did not cap")
	}
	if err := New(func(ctx context.Context) error { return nil }).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCronRangesStepsAndDayRules(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := (Schedule{Cron: "*/15 0 * * *", Timezone: "UTC"}).Next(now)
	if err != nil || next.Minute() != 15 {
		t.Fatalf("step cron failed: %v %v", next, err)
	}
	next, err = (Schedule{Cron: "0 9 1-5 * *", Timezone: "UTC"}).Next(now)
	if err != nil || next.Day() != 1 || next.Hour() != 9 {
		t.Fatalf("range cron failed: %v %v", next, err)
	}
	if _, err := (Schedule{Cron: "0 0 1-99 * *", Timezone: "UTC"}).Next(now); err == nil {
		t.Fatal("out-of-range cron was accepted")
	}
}

func TestScheduleSupportsWholeHourUTCOffsets(t *testing.T) {
	next, err := (Schedule{Cron: "0 0 * * *", Timezone: "UTC+23:00"}).Next(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, offset := next.Zone(); offset != 23*60*60 {
		t.Fatalf("offset = %d, want %d", offset, 23*60*60)
	}
}
