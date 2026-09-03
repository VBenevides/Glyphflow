package controlplane

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type coverageScheduleErrorRepository struct{}

func (coverageScheduleErrorRepository) CreateDueRun(context.Context, time.Time, func(store.DueScheduleRecord) (time.Time, error)) (string, bool, error) {
	return "", false, errors.New("schedule storage unavailable")
}

func TestScheduleAndPolicyEdgeCoverage(t *testing.T) {
	if err := RunScheduler(context.Background(), coverageScheduleErrorRepository{}, time.Minute); err == nil {
		t.Fatal("scheduler storage error was ignored")
	}
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	testScheduleEdges(t, now)
	testPolicyEdges(t, now)
	testProjectionEdges(t, now)
	testDueScheduleQueue(t, now)
}

func testScheduleEdges(t *testing.T, now time.Time) {
	if got, err := (Schedule{Manual: true}).Next(now); err != nil || !got.Equal(now) {
		t.Fatalf("manual schedule = %v, %v", got, err)
	}
	if _, err := (Schedule{Timezone: "not/a-timezone", Cron: "* * * * *"}).Next(now); err == nil {
		t.Fatal("invalid timezone accepted")
	}
	past := now.Add(-time.Minute)
	if _, err := (Schedule{At: &past}).Next(now); err == nil {
		t.Fatal("past fixed schedule accepted")
	}
	future := now.Add(time.Hour)
	if got, err := (Schedule{At: &future}).Next(now); err != nil || !got.Equal(future) {
		t.Fatalf("future fixed schedule = %v, %v", got, err)
	}
	if got, err := (Schedule{Cron: "0 13 * * *"}).Next(now); err != nil || got.Hour() != 13 {
		t.Fatalf("cron schedule = %v, %v", got, err)
	}
	for _, expression := range []string{"", "* * *", "bad * * * *"} {
		if _, err := (Schedule{Cron: expression}).Next(now); err == nil {
			t.Fatalf("invalid schedule %q accepted", expression)
		}
	}
	for _, expression := range []string{"bad * * * *", "* bad * * *", "* * bad * *", "* * * bad *", "* * * * bad"} {
		if _, _, _, _, _, err := parseCronFields(expression); err == nil {
			t.Fatalf("invalid cron field %q accepted", expression)
		}
	}
	if _, err := nextCronMinute(now, "0 0 30 2 *"); err == nil {
		t.Fatal("cron without a calendar date accepted")
	}
}

func testPolicyEdges(t *testing.T, now time.Time) {
	valid := SchedulePolicy{Misfire: MisfireRunLatest, Concurrency: ConcurrencyQueue, StartDeadline: time.Minute, ExecutionTimeout: time.Minute}
	for _, policy := range []SchedulePolicy{
		{Misfire: "bad", Concurrency: ConcurrencyQueue, ExecutionTimeout: time.Minute},
		{Misfire: MisfireRunLatest, Concurrency: "bad", ExecutionTimeout: time.Minute},
		{Misfire: MisfireRunLatest, Concurrency: ConcurrencyQueue, MaxConcurrentRuns: 1, ExecutionTimeout: time.Minute},
		{Misfire: MisfireRunLatest, Concurrency: ConcurrencyAllow, ExecutionTimeout: time.Minute},
		{Misfire: MisfireRunLatest, Concurrency: ConcurrencyQueue, StartDeadline: -time.Second, ExecutionTimeout: time.Minute},
		{Misfire: MisfireRunLatest, Concurrency: ConcurrencyQueue, ExecutionTimeout: 0},
		{Misfire: MisfireRunLatest, Concurrency: ConcurrencyQueue, CatchUpLimit: -1, ExecutionTimeout: time.Minute},
	} {
		if err := policy.Validate(); err == nil {
			t.Fatalf("invalid policy accepted: %#v", policy)
		}
	}
	if count, err := valid.OccurrencesDue(now, now, time.Minute, 0); err != nil || count != 0 {
		t.Fatalf("not-due policy = %d, %v", count, err)
	}
	if count, err := valid.OccurrencesDue(now, now.Add(-time.Minute), 0, 0); err != nil || count != 0 {
		t.Fatalf("zero-interval policy = %d, %v", count, err)
	}
	for _, misfire := range []MisfirePolicy{MisfireSkipAll, MisfireRunLatest, MisfireFailAndAlert} {
		policy := valid
		policy.Misfire = misfire
		decision, err := policy.Evaluate(now, now.Add(-2*time.Minute), time.Minute, 0)
		if err != nil || (misfire == MisfireFailAndAlert && (!decision.Failed || decision.Occurrences != 0)) {
			t.Fatalf("evaluate %s = %#v, %v", misfire, decision, err)
		}
	}
	for _, concurrency := range []ConcurrencyPolicy{ConcurrencyQueue, ConcurrencySkip, ConcurrencyReplace, ConcurrencyAllow, "bad"} {
		policy := valid
		policy.Concurrency = concurrency
		policy.MaxConcurrentRuns = 2
		if concurrency != ConcurrencyAllow {
			policy.MaxConcurrentRuns = 0
		}
		_ = policy.AllowsConcurrency(1)
	}
	for _, concurrency := range []ConcurrencyPolicy{ConcurrencySkip, ConcurrencyReplace} {
		policy := valid
		policy.Concurrency = concurrency
		decision, err := policy.Evaluate(now, now.Add(-time.Minute), time.Minute, 1)
		if err != nil || (concurrency == ConcurrencySkip && !decision.Skipped) || (concurrency == ConcurrencyReplace && !decision.Replaced) {
			t.Fatalf("concurrency decision %s = %#v, %v", concurrency, decision, err)
		}
	}
	if !valid.DeadlineExceeded(now, now.Add(time.Minute+time.Nanosecond)) {
		t.Fatal("deadline was not exceeded")
	}
}

func testProjectionEdges(t *testing.T, now time.Time) {
	if _, err := BuildScheduleProjection(nil, time.Time{}); err == nil {
		t.Fatal("zero-time projection accepted")
	}
	if _, err := BuildScheduleProjection([]store.ScheduleProjectionInput{{}}, now); err == nil {
		t.Fatal("incomplete projection accepted")
	}
	if _, err := BuildScheduleProjection([]store.ScheduleProjectionInput{{ScheduleID: "s", TaskID: "t", TaskVersionID: "v", Expression: "* * * * *"}}, now); err == nil {
		t.Fatal("zero-duration projection accepted")
	}
	if _, err := BuildScheduleProjection([]store.ScheduleProjectionInput{{ScheduleID: "s", TaskID: "t", TaskVersionID: "v", Expression: "invalid", DurationSeconds: 1}}, now); err == nil {
		t.Fatal("invalid projection schedule accepted")
	}
	if signSecretDeliveryResponse(protocol.SecretDeliveryResponse{Version: protocol.ProtocolVersion}, protocol.SigningKey{ID: "invalid"}) != nil {
		t.Fatal("response signed with invalid key")
	}
	if secretDeliveryResponse(context.Background(), nil, nil, nil, protocol.SigningKey{ID: "invalid"}, nil, []byte("invalid")) != nil {
		t.Fatal("rejected response signed with invalid key")
	}
}

func testDueScheduleQueue(t *testing.T, now time.Time) {
	queue := NewDueScheduleQueue()
	queue.Add(DueSchedule{})
	queue.Add(DueSchedule{ID: "invalid", Interval: 0})
	if _, ok := queue.Claim(now); ok {
		t.Fatal("empty due queue claimed a schedule")
	}
	queue.Add(DueSchedule{ID: "future", NextFire: now.Add(time.Hour), Interval: time.Minute})
	if _, ok := queue.Claim(now); ok {
		t.Fatal("future schedule was claimed")
	}
	queue.Add(DueSchedule{ID: "due", NextFire: now.Add(-2 * time.Minute), Interval: time.Minute})
	claimed, ok := queue.Claim(now)
	if !ok || !claimed.OccurrenceAt.Equal(now.Add(-2*time.Minute)) || !claimed.NextFire.After(now) {
		t.Fatalf("due schedule = %#v, claimed %v", claimed, ok)
	}
}
