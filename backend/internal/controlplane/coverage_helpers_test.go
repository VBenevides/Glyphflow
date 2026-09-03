package controlplane

import (
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

func TestCronHelperBranches(t *testing.T) {
	if got, err := parseCronField("*/15", 0, 59); err != nil || len(got) != 4 || !got[45] {
		t.Fatalf("parseCronField = %#v, %v", got, err)
	}
	for _, field := range []string{"", "*/0", "*/x", "x-y", "5-2", "x", "60"} {
		if _, err := parseCronField(field, 0, 59); err == nil {
			t.Errorf("parseCronField(%q) accepted invalid input", field)
		}
	}
	if got := splitComma("a,b,"); len(got) != 3 || got[2] != "" {
		t.Fatalf("splitComma = %#v", got)
	}
	if !cronFieldIsAny(map[int]bool{0: true, 1: true}, 0, 1) {
		t.Fatal("cronFieldIsAny rejected a complete field")
	}
	if cronHasCalendarDate(map[int]bool{30: true}, map[int]bool{2: true}) {
		t.Fatal("cronHasCalendarDate accepted February 30")
	}

	candidate := time.Date(2026, time.September, 3, 12, 34, 0, 0, time.UTC)
	minute := map[int]bool{candidate.Minute(): true}
	hour := map[int]bool{candidate.Hour(): true}
	month := map[int]bool{int(candidate.Month()): true}
	dom := map[int]bool{candidate.Day(): true}
	dow := map[int]bool{int(candidate.Weekday()): true}
	for _, test := range []struct {
		domAny, dowAny, want bool
	}{
		{true, true, true}, {true, false, true}, {false, true, true}, {false, false, true}, {false, false, false},
	} {
		candidateDOM, candidateDOW := dom, dow
		if !test.want {
			candidateDOW = map[int]bool{}
			candidateDOM = map[int]bool{}
		}
		calendar := cronCalendar{dom: candidateDOM, month: month, dow: candidateDOW, domAny: test.domAny, dowAny: test.dowAny}
		if got := cronMinuteMatches(candidate, minute, hour, calendar); got != test.want {
			t.Errorf("cronMinuteMatches(%+v) = %v, want %v", test, got, test.want)
		}
	}
}

func TestProjectionHelperBranches(t *testing.T) {
	if got := fallbackName("  ", "fallback-id"); got != "fallback-id" {
		t.Fatalf("fallbackName = %q", got)
	}
	if id, label := projectionLane(store.ScheduleProjectionInput{PinnedRunnerID: "runner-1"}); id != "runner:runner-1" || label != "Runner: runner-1" {
		t.Fatalf("pinned projection lane = %q, %q", id, label)
	}
	if id, label := projectionLane(store.ScheduleProjectionInput{RunnerPoolID: "pool-1", RunnerPoolName: "Pool"}); id != "pool:pool-1" || label != "Any runner in Pool" {
		t.Fatalf("pool projection lane = %q, %q", id, label)
	}
	resources := exclusiveResources([]store.ScheduleProjectionResource{
		{ID: "b", Name: "B", Kind: "exclusive"},
		{ID: "a", Kind: "EXCLUSIVE"},
		{ID: "b", Name: "duplicate", Kind: "exclusive"},
		{ID: "shared", Name: "Shared", Kind: "shared"},
		{Kind: "exclusive"},
	})
	if len(resources) != 2 || resources[0].ID != "a" || resources[0].Name != "a" || resources[1].ID != "b" {
		t.Fatalf("exclusiveResources = %#v", resources)
	}

	start := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	occurrence := ProjectionOccurrence{ID: "one", ScheduleID: "schedule", ScheduleName: "Schedule", StartAt: start, EndAt: start.Add(time.Hour), LaneLabel: "Pool"}
	windows := []projectionWindow{{occurrence: occurrence, resources: resources}}
	segments := projectionSegments(windows, map[string]bool{"one": true})
	if len(segments) != 1 || !segments[0].Conflicted || len(segments[0].ExclusiveResources) != 2 {
		t.Fatalf("projectionSegments = %#v", segments)
	}
}

func TestHeartbeatMetricValidation(t *testing.T) {
	if got, err := heartbeatMetrics(runnerHeartbeat{}); got != nil || err != nil {
		t.Fatalf("empty heartbeat metrics = %#v, %v", got, err)
	}
	cpu := 10.0
	if got, err := heartbeatMetrics(runnerHeartbeat{CPUPercent: &cpu}); got != nil || err == nil {
		t.Fatalf("incomplete heartbeat metrics = %#v, %v", got, err)
	}
	memory := 20.0
	used, total := int64(10), int64(100)
	got, err := heartbeatMetrics(runnerHeartbeat{CPUPercent: &cpu, MemoryPercent: &memory, MemoryUsedBytes: &used, MemoryTotalBytes: &total})
	if err != nil || got == nil || got.CPUPercent != cpu || got.MemoryTotalBytes != total {
		t.Fatalf("complete heartbeat metrics = %#v, %v", got, err)
	}
}
