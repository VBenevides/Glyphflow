package api

import (
	"testing"
	"time"
)

func TestPreviewOccurrencesReturnsFiveIncreasingTimes(t *testing.T) {
	items, err := previewOccurrences("*/5 * * * *", "UTC", time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC))
	if err != nil || len(items) != 5 {
		t.Fatalf("preview = %#v, err = %v", items, err)
	}
	for i := 1; i < len(items); i++ {
		if items[i] <= items[i-1] {
			t.Fatalf("preview is not increasing: %#v", items)
		}
	}
}

func TestTaskDefinitionIncludesResourceRequirements(t *testing.T) {
	definition := taskDefinition("task-1", taskInput{Name: "Build", RunnerPool: "default", Command: []string{"echo", "ok"}, Resources: []string{"resource-1", "resource-2"}})
	if len(definition.ResourceIDs) != 2 || definition.ResourceIDs[0] != "resource-1" || definition.ResourceIDs[1] != "resource-2" {
		t.Fatalf("resource IDs = %#v", definition.ResourceIDs)
	}
}
