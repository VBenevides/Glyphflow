package controlplane

import (
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

func TestBuildScheduleProjectionUsesTimeoutAndPlacement(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	report, err := BuildScheduleProjection([]store.ScheduleProjectionInput{{
		ScheduleID: "schedule-1", ScheduleName: "Hourly", ScheduleVersionID: "schedule-1-v1", TaskID: "task-1", TaskName: "Backup", TaskVersionID: "task-1-v1", Expression: "0 * * * *", Timezone: "UTC", RunnerPoolID: "pool-1", RunnerPoolName: "Build", PinnedRunnerID: "runner-1", PinnedRunnerName: "Runner A", TimeoutSeconds: 90,
	}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Available || len(report.Segments) != 167 || report.Segments[0].LaneLabel != "Runner: Runner A" || report.Segments[0].EndAt.Sub(report.Segments[0].StartAt) != 90*time.Second {
		t.Fatalf("report = %#v", report)
	}
}

func TestBuildScheduleProjectionGroupsExclusiveConflicts(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	inputs := []store.ScheduleProjectionInput{
		{ScheduleID: "schedule-a", ScheduleName: "A", ScheduleVersionID: "schedule-a-v1", TaskID: "task-a", TaskName: "A", TaskVersionID: "task-a-v1", Expression: "0 * * * *", Timezone: "UTC", RunnerPoolID: "pool", RunnerPoolName: "Pool", TimeoutSeconds: 3600, Resources: []store.ScheduleProjectionResource{{ID: "resource-1", Name: "Database", Kind: "exclusive"}}},
		{ScheduleID: "schedule-b", ScheduleName: "B", ScheduleVersionID: "schedule-b-v1", TaskID: "task-b", TaskName: "B", TaskVersionID: "task-b-v1", Expression: "30 * * * *", Timezone: "UTC", RunnerPoolID: "pool", RunnerPoolName: "Pool", TimeoutSeconds: 1800, Resources: []store.ScheduleProjectionResource{{ID: "resource-1", Name: "Database", Kind: "exclusive"}}},
		{ScheduleID: "schedule-c", ScheduleName: "C", ScheduleVersionID: "schedule-c-v1", TaskID: "task-c", TaskName: "C", TaskVersionID: "task-c-v1", Expression: "30 * * * *", Timezone: "UTC", RunnerPoolID: "pool", RunnerPoolName: "Pool", TimeoutSeconds: 1800, Resources: []store.ScheduleProjectionResource{{ID: "resource-2", Name: "Shared", Kind: "non_blocking"}}},
	}
	report, err := BuildScheduleProjection(inputs, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Conflicts) != 167 {
		t.Fatalf("conflicts = %d, want 167", len(report.Conflicts))
	}
	if report.Conflicts[0].ResourceID != "resource-1" || len(report.Conflicts[0].Occurrences) < 2 {
		t.Fatalf("first conflict = %#v", report.Conflicts[0])
	}
	for _, segment := range report.Segments {
		if segment.ScheduleID == "schedule-c" && segment.Conflicted {
			t.Fatal("non-blocking resource was marked conflicted")
		}
	}
}

func TestBuildScheduleProjectionTreatsAdjacentWindowsAsNonConflicting(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	inputs := []store.ScheduleProjectionInput{
		{ScheduleID: "schedule-a", ScheduleVersionID: "schedule-a-v1", TaskID: "task-a", TaskVersionID: "task-a-v1", Expression: "0 * * * *", Timezone: "UTC", RunnerPoolID: "pool", TimeoutSeconds: 1800, Resources: []store.ScheduleProjectionResource{{ID: "resource-1", Kind: "exclusive"}}},
		{ScheduleID: "schedule-b", ScheduleVersionID: "schedule-b-v1", TaskID: "task-b", TaskVersionID: "task-b-v1", Expression: "30 * * * *", Timezone: "UTC", RunnerPoolID: "pool", TimeoutSeconds: 1800, Resources: []store.ScheduleProjectionResource{{ID: "resource-1", Kind: "exclusive"}}},
	}
	report, err := BuildScheduleProjection(inputs, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Conflicts) != 0 {
		t.Fatalf("adjacent conflicts = %#v", report.Conflicts)
	}
}
