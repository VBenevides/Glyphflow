package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRunActionsAndResumableLogs(t *testing.T) {
	runs := NewRunService()
	created := httptest.NewRecorder()
	runs.execute(created, httptest.NewRequest(http.MethodPost, "/api/v1/runs/execute", bytes.NewBufferString(`{"task_id":"task-1"}`)))
	if created.Code != http.StatusCreated {
		t.Fatalf("execute status = %d", created.Code)
	}
	var run RunRecord
	if err := json.NewDecoder(created.Body).Decode(&run); err != nil {
		t.Fatal(err)
	}
	if len(run.ID) != len("run-")+128 {
		t.Fatalf("run ID length = %d", len(run.ID))
	}
	if err := func() error {
		runs.mu.Lock()
		defer runs.mu.Unlock()
		runs.logs[run.ID]["stdout"] = []LogChunk{{Sequence: 1, Text: "hello\n"}, {Sequence: 2, Text: "world\n"}}
		return nil
	}(); err != nil {
		t.Fatal(err)
	}
	cancel := httptest.NewRecorder()
	runs.path(cancel, httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+run.ID+"/cancel", bytes.NewBufferString(`{"reason":"stop"}`)))
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d", cancel.Code)
	}
	logs := httptest.NewRecorder()
	runs.path(logs, httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+run.ID+"/logs?stream=stdout&after=1", nil))
	if logs.Code != http.StatusOK || !bytes.Contains(logs.Body.Bytes(), []byte(`"sequence":2`)) || bytes.Contains(logs.Body.Bytes(), []byte(`"sequence":1`)) {
		t.Fatalf("resumed logs: %d %s", logs.Code, logs.Body.String())
	}
	conflict := httptest.NewRecorder()
	runs.path(conflict, httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+run.ID+"/retry", bytes.NewBufferString(`{"reason":"repeat"}`)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("illegal retry status = %d", conflict.Code)
	}
}

func TestRunCollectionMatchesRunnerExactly(t *testing.T) {
	items := filterRuns([]RunRecord{{ID: "one", Runner: "runner-1"}, {ID: "ten", Runner: "runner-10"}}, url.Values{"runner": []string{"runner-1"}})
	if len(items) != 1 || items[0].ID != "one" {
		t.Fatalf("runner filter = %#v", items)
	}
}

func TestRunCollectionFiltersActiveRuns(t *testing.T) {
	runs := NewRunService()
	runs.runs = map[string]RunRecord{
		"waiting":   {ID: "waiting", State: "WAITING"},
		"cancelled": {ID: "cancelled", State: "CANCELLED"},
	}
	response := httptest.NewRecorder()
	runs.collection(response, httptest.NewRequest(http.MethodGet, "/api/v1/runs?state=active", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list status = %d", response.Code)
	}
	var page struct {
		Items []RunRecord `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != "waiting" {
		t.Fatalf("active runs = %#v", page.Items)
	}
}
