package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type runPageRepositoryStub struct {
	store.RunRepository
	filter        store.RunListFilter
	listPageCalls int
}

type runLogRepositoryStub struct {
	store.RunRepository
	err    error
	chunks []store.RunLogChunkRecord
}

func (s runLogRepositoryStub) ListLogChunks(context.Context, string, string, int64) ([]store.RunLogChunkRecord, error) {
	return s.chunks, s.err
}

func (s *runPageRepositoryStub) ListPage(_ context.Context, filter store.RunListFilter) (store.RunPage, error) {
	s.filter = filter
	s.listPageCalls++
	return store.RunPage{}, nil
}

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

func TestRunLogsRejectBudgetOverflow(t *testing.T) {
	runs := NewRunService()
	runs.SetRepository(runLogRepositoryStub{err: store.ErrRunLogBudgetExceeded})
	response := httptest.NewRecorder()
	runs.path(response, httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/logs?stream=stdout", nil))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestInMemoryRunLogDownloadResumesAfterSequence(t *testing.T) {
	runs := NewRunService()
	runs.runs["run-1"] = RunRecord{ID: "run-1"}
	runs.logs["run-1"] = map[string][]LogChunk{"stdout": {{Sequence: 1, Text: "old\n"}, {Sequence: 2, Text: "new\n"}}}
	response := httptest.NewRecorder()
	runs.path(response, httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/logs/download?stream=stdout&after=1", nil))
	if response.Code != http.StatusOK || response.Body.String() != "new\n" {
		t.Fatalf("status = %d, body = %q", response.Code, response.Body.String())
	}
}

func TestRunRecordIncludesExecutionError(t *testing.T) {
	run := runRecordFromStore(store.RunRecord{ID: "run-1", TaskVersionID: "task-1-v2", ScheduleID: "schedule-1", ScheduleVersionID: "schedule-1-v3", Error: "exec: file not found"})
	if run.Error != "exec: file not found" {
		t.Fatalf("run error = %q", run.Error)
	}
	if run.TaskVersionID != "task-1-v2" || run.ScheduleID != "schedule-1" || run.ScheduleVersionID != "schedule-1-v3" {
		t.Fatalf("run version references = %#v", run)
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
		"waiting":    {ID: "waiting", State: "WAITING"},
		"dispatched": {ID: "dispatched", State: "DISPATCHED"},
		"cancelled":  {ID: "cancelled", State: "CANCELLED"},
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
	if len(page.Items) != 2 {
		t.Fatalf("active runs = %#v", page.Items)
	}
}

func TestRunListFilterKeepsPageRequestsBounded(t *testing.T) {
	filter, page, limit := runListFilter(httptest.NewRequest(http.MethodGet, "/api/v1/runs?all=true&page=2&limit=1000&task=client", nil))
	if page != 1 || limit != 100 || filter.Limit != 100 || filter.Offset != 0 {
		t.Fatalf("run page filter = %#v page=%d limit=%d", filter, page, limit)
	}
}

func TestRunCollectionPreservesNormalAndClampedPagination(t *testing.T) {
	for _, test := range []struct {
		name                string
		path                string
		page, limit, offset int
	}{
		{name: "normal", path: "/api/v1/runs?page=2&limit=10", page: 2, limit: 10, offset: 10},
		{name: "clamped", path: "/api/v1/runs?page=0&limit=1000", page: 1, limit: 50, offset: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &runPageRepositoryStub{}
			runs := NewRunService()
			runs.SetRepository(repository)
			response := httptest.NewRecorder()
			runs.collection(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK || repository.listPageCalls != 1 {
				t.Fatalf("status=%d list page calls=%d body=%s", response.Code, repository.listPageCalls, response.Body.String())
			}
			if repository.filter.Limit != test.limit || repository.filter.Offset != test.offset {
				t.Fatalf("filter=%#v, want page=%d limit=%d offset=%d", repository.filter, test.page, test.limit, test.offset)
			}
		})
	}
}

func TestRunCollectionRejectsOverflowingPaginationOffset(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, page := range []int{maxInt/100 + 2, maxInt} {
		repository := &runPageRepositoryStub{}
		runs := NewRunService()
		runs.SetRepository(repository)
		response := httptest.NewRecorder()
		path := "/api/v1/runs?page=" + strconv.Itoa(page) + "&limit=100"
		runs.collection(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusBadRequest || !bytes.Contains(response.Body.Bytes(), []byte(paginationOffsetError)) {
			t.Fatalf("page=%d response=%d body=%s", page, response.Code, response.Body.String())
		}
		if repository.listPageCalls != 0 {
			t.Fatalf("page=%d reached the store", page)
		}
	}
}
