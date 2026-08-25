package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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

func TestTaskSecretReferencesAreRejectedAtTheAPI(t *testing.T) {
	o := NewOperationsService()
	create := httptest.NewRecorder()
	o.taskCollection(create, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"name":"secret task","command":["echo"],"runner_pool":"default","secret_references":{"TOKEN":"env://TOKEN"}}`)))
	if create.Code != http.StatusBadRequest || !bytes.Contains(create.Body.Bytes(), []byte("secret references are not supported")) {
		t.Fatalf("task creation accepted secret references: %d %s", create.Code, create.Body.String())
	}

	version := httptest.NewRecorder()
	o.taskPath(version, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/versions", bytes.NewBufferString(`{"command":["echo"],"secret_references":{"TOKEN":"env://TOKEN"}}`)))
	if version.Code != http.StatusBadRequest || !bytes.Contains(version.Body.Bytes(), []byte("secret references are not supported")) {
		t.Fatalf("task version accepted secret references: %d %s", version.Code, version.Body.String())
	}
}
