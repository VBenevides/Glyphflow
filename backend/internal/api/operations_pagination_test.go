package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type paginationTaskRepository struct {
	items []store.TaskRecord
}

func (r paginationTaskRepository) List(context.Context, bool) ([]store.TaskRecord, error) {
	return r.items, nil
}

func (paginationTaskRepository) Find(context.Context, string) (store.TaskRecord, bool, error) {
	return store.TaskRecord{}, false, nil
}

func (paginationTaskRepository) ListVersions(context.Context, string) ([]store.TaskVersionRecord, error) {
	return nil, nil
}

func (paginationTaskRepository) Create(context.Context, store.TaskDefinition) (store.TaskRecord, error) {
	return store.TaskRecord{}, nil
}

func (paginationTaskRepository) CreateVersion(context.Context, string, store.TaskDefinition) (store.TaskRecord, error) {
	return store.TaskRecord{}, nil
}

func (paginationTaskRepository) Delete(context.Context, string) (bool, error) {
	return false, nil
}

func TestTaskCollectionBoundsPaginationPage(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	cases := []struct {
		name      string
		page      string
		wantPage  int
		wantItems int
	}{
		{name: "maximum integer", page: strconv.Itoa(maxInt), wantPage: maxInt},
		{name: "negative", page: "-1", wantPage: 1, wantItems: 1},
		{name: "malformed", page: "not-a-number", wantPage: 1, wantItems: 1},
		{name: "very large", page: strconv.Itoa(maxInt / 100), wantPage: maxInt / 100},
	}

	for _, source := range []struct {
		name    string
		service func() *OperationsService
	}{
		{name: "in-memory", service: func() *OperationsService {
			o := NewOperationsService()
			o.tasks["task-1"] = TaskRecord{ID: "task-1"}
			return o
		}},
		{name: "repository", service: func() *OperationsService {
			o := NewOperationsService()
			o.SetTaskRepository(paginationTaskRepository{items: []store.TaskRecord{{ID: "task-1"}}})
			return o
		}},
	} {
		for _, test := range cases {
			t.Run(source.name+"/"+test.name, func(t *testing.T) {
				response := httptest.NewRecorder()
				source.service().taskCollection(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?page="+test.page, nil))
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
				}
				var page struct {
					Items []json.RawMessage `json:"items"`
					Page  int               `json:"page"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
					t.Fatal(err)
				}
				if page.Page != test.wantPage || len(page.Items) != test.wantItems {
					t.Fatalf("page = %d, items = %d, want page %d, items %d", page.Page, len(page.Items), test.wantPage, test.wantItems)
				}
			})
		}
	}
}
