package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
)

func TestSharedContractFixtures(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "task", value: contractTaskFixture()},
		{name: "run", value: contractRunFixture()},
		{name: "schedule-projection", value: contractProjectionFixture()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture, err := os.ReadFile(contractFixturePath(test.name + ".json"))
			if err != nil {
				t.Fatal(err)
			}
			var expected, actual any
			if err := json.Unmarshal(fixture, &expected); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &actual); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(expected, actual) {
				t.Fatalf("fixture mismatch\nexpected: %s\nactual: %s", fixture, encoded)
			}
		})
	}
}

func contractFixturePath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "contracts", "fixtures", name)
}

func contractTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func contractTaskFixture() TaskRecord {
	return TaskRecord{
		ID: "task-1", Name: "Nightly backup", Enabled: true, ActiveVersion: 2, Pool: "default", IsDeleted: false,
		PinnedRunner: "runner-1", Command: []string{"/bin/echo", "hello"}, WorkingDirectory: "/tmp",
		PlacementSelectors: map[string]any{"os": "linux"}, Environment: map[string]any{"MODE": "test"},
		SecretReferences: map[string]any{"token": "env://TOKEN"}, DurationSeconds: 30, MaxOutputBytes: 1048576,
		MaxAttempts: 3, AmbiguityPolicy: "RETRY", Resources: []string{"resource-1"},
	}
}

func contractRunFixture() RunRecord {
	exitCode := 0
	return RunRecord{
		ID: "run-1", TaskID: "task-1", TaskVersionID: "task-1-v2", ScheduleID: "schedule-1", ScheduleVersionID: "schedule-1-v1",
		TaskName: "Nightly backup", State: "SUCCEEDED", Attempt: 2, ExitCode: &exitCode, ExitCodeMeaning: "Success",
		Runner: "runner-1", Trigger: "SCHEDULE", ScheduledFor: "2026-08-27T00:00:00Z", MaxMemoryUsed: 123456, AverageMemoryUsed: 65432,
	}
}

func contractProjectionFixture() controlplane.ProjectionReport {
	return controlplane.ProjectionReport{
		Available: true, CalculatedAt: contractTime("2026-08-27T00:00:00Z"), WindowStart: contractTime("2026-08-27T00:00:00Z"),
		WindowEnd: contractTime("2026-09-03T00:00:00Z"), DurationSource: "task_duration",
		Segments: []controlplane.ProjectionSegment{{
			ID: "schedule-1:2026-08-27T00:00:00Z", ScheduleID: "schedule-1", ScheduleName: "Nightly backup schedule", ScheduleVersionID: "schedule-1-v1",
			TaskID: "task-1", TaskName: "Nightly backup", TaskVersionID: "task-1-v2", Timezone: "UTC", LaneID: "pool:default", LaneLabel: "default",
			StartAt: contractTime("2026-08-27T00:00:00Z"), EndAt: contractTime("2026-08-27T00:00:30Z"), OccurrenceCount: 1,
			ExclusiveResources: []controlplane.ProjectionResource{{ID: "resource-1", Name: "Production database"}},
		}},
		Conflicts: []controlplane.ProjectionConflict{},
	}
}
