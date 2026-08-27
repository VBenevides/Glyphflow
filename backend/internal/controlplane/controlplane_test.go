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
	next, err = (Schedule{Cron: "0 0 1-31 2 1", Timezone: "UTC"}).Next(time.Date(2026, 2, 2, 0, 1, 0, 0, time.UTC))
	if err != nil || !next.Equal(time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("full DOM field was not treated as any: %v %v", next, err)
	}
	next, err = (Schedule{Cron: "1/2 0 * * *", Timezone: "UTC"}).Next(time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC))
	if err != nil || !next.Equal(time.Date(2026, 1, 1, 0, 3, 0, 0, time.UTC)) {
		t.Fatalf("numeric step cron failed: %v %v", next, err)
	}
}

func TestCronCalendarAndDSTBoundaries(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	next, err := (Schedule{Cron: "30 2 * * *", Timezone: "America/New_York"}).Next(time.Date(2026, 3, 8, 1, 59, 0, 0, newYork))
	if err != nil || !next.Equal(time.Date(2026, 3, 9, 2, 30, 0, 0, newYork)) {
		t.Fatalf("DST gap = %v %v", next, err)
	}
	next, err = (Schedule{Cron: "0 0 29 2 *", Timezone: "UTC"}).Next(time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || !next.Equal(time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("leap day = %v %v", next, err)
	}
	first, err := (Schedule{Cron: "30 1 * * *", Timezone: "America/New_York"}).Next(time.Date(2026, 11, 1, 0, 0, 0, 0, newYork))
	if err != nil {
		t.Fatal(err)
	}
	second, err := (Schedule{Cron: "30 1 * * *", Timezone: "America/New_York"}).Next(first)
	if err != nil || !second.Equal(first.Add(time.Hour)) || second.Format("-0700") != "-0500" {
		t.Fatalf("DST fold = %v %v after %v", second, err, first)
	}
	if _, err := NextFire("0 0 31 2 *", "UTC", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("impossible calendar date was accepted")
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

func TestNextFireSupportsCron(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := NextFire("*/5 * * * *", "UTC", now)
	if err != nil || !next.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("cron next fire = %v, err=%v", next, err)
	}
}

func BenchmarkNextFireCron(b *testing.B) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, expression := range map[string]string{
		"dense":         "*/5 * * * *",
		"sparse":        "0 9 1-5 * *",
		"unsatisfiable": "0 0 31 2 *",
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				_, _ = NextFire(expression, "UTC", now)
			}
		})
	}
}
