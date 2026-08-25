package store

import (
	"strings"
	"testing"
	"time"
)

func TestRunListWhereUsesBoundParametersForEveryFilter(t *testing.T) {
	where, args := runListWhere(RunListFilter{
		State:   "ACTIVE",
		Task:    "client task",
		Runner:  "runner-1",
		Trigger: "MANUAL",
		From:    time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		To:      time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC),
	})
	if strings.Contains(where, "?") || !strings.Contains(where, "r.state IN") {
		t.Fatalf("unsafe or missing filter SQL: %s", where)
	}
	if len(args) != 6 || args[0] != "%client task%" || args[1] != "%client task%" {
		t.Fatalf("filter args = %#v", args)
	}
}
