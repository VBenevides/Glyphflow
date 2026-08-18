package api

import (
	"net/url"
	"testing"
)

func TestOperationsStateFilters(t *testing.T) {
	schedules := []ScheduleRecord{{ID: "enabled", Enabled: true}, {ID: "disabled", Enabled: false}}
	if got := filterSchedules(schedules, url.Values{"enabled": {"false"}}); len(got) != 1 || got[0].ID != "disabled" {
		t.Fatalf("disabled schedules = %#v", got)
	}

	tasks := []TaskRecord{{ID: "enabled", Enabled: true}, {ID: "disabled", Enabled: false}}
	if got := filterTasks(tasks, url.Values{"state": {"disabled"}}); len(got) != 1 || got[0].ID != "disabled" {
		t.Fatalf("disabled tasks = %#v", got)
	}
}
