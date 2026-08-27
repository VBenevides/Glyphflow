package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type conflictProjectionRepository func(context.Context) ([]store.ScheduleProjectionInput, error)

func (f conflictProjectionRepository) ListScheduleProjection(ctx context.Context) ([]store.ScheduleProjectionInput, error) {
	return f(ctx)
}

type conflictTaskRepository struct{ task store.TaskRecord }

func (r conflictTaskRepository) List(context.Context, bool) ([]store.TaskRecord, error) {
	return []store.TaskRecord{r.task}, nil
}
func (r conflictTaskRepository) Find(context.Context, string) (store.TaskRecord, bool, error) {
	return r.task, true, nil
}
func (r conflictTaskRepository) ListVersions(context.Context, string) ([]store.TaskVersionRecord, error) {
	return nil, nil
}
func (r conflictTaskRepository) Create(context.Context, store.TaskDefinition) (store.TaskRecord, error) {
	return r.task, nil
}
func (r conflictTaskRepository) CreateVersion(context.Context, string, store.TaskDefinition) (store.TaskRecord, error) {
	return r.task, nil
}
func (r conflictTaskRepository) Delete(context.Context, string) (bool, error) { return false, nil }

type conflictResourceRepository struct{}

func (conflictResourceRepository) List(context.Context) ([]store.ResourceRecord, error) {
	return nil, nil
}
func (conflictResourceRepository) Find(context.Context, string) (store.ResourceRecord, bool, error) {
	return store.ResourceRecord{ID: "resource-1", Name: "Database", Kind: "exclusive", Enabled: true}, true, nil
}
func (conflictResourceRepository) Create(context.Context, string, string, string) error { return nil }
func (conflictResourceRepository) Delete(context.Context, string) error                 { return nil }
func (conflictResourceRepository) Acquire(context.Context, string, string, time.Duration, time.Time) (store.ResourceRecord, error) {
	return store.ResourceRecord{}, nil
}
func (conflictResourceRepository) Release(context.Context, string, string, int64) error { return nil }

type conflictScheduleRepository struct{ created bool }

func (r *conflictScheduleRepository) List(context.Context) ([]store.ScheduleRecord, error) {
	return nil, nil
}
func (r *conflictScheduleRepository) Find(context.Context, string) (store.ScheduleRecord, bool, error) {
	return store.ScheduleRecord{}, false, nil
}
func (r *conflictScheduleRepository) Create(_ context.Context, definition store.ScheduleDefinition) (store.ScheduleRecord, error) {
	r.created = true
	return store.ScheduleRecord{ID: definition.ID, Name: definition.Name, TaskID: definition.TaskID}, nil
}
func (r *conflictScheduleRepository) Update(context.Context, string, store.ScheduleDefinition) (store.ScheduleRecord, error) {
	return store.ScheduleRecord{}, nil
}
func (r *conflictScheduleRepository) SetEnabled(context.Context, string, bool) (store.ScheduleRecord, bool, error) {
	return store.ScheduleRecord{}, false, nil
}
func (r *conflictScheduleRepository) Delete(context.Context, string) (bool, error) { return false, nil }
func (r *conflictScheduleRepository) CreateDueRun(context.Context, time.Time, func(store.DueScheduleRecord) (time.Time, error)) (string, bool, error) {
	return "", false, nil
}

func TestScheduleCreationRejectsExclusiveResourceConflict(t *testing.T) {
	scheduleRepository := &conflictScheduleRepository{}
	service := controlplane.NewProjectionService(conflictProjectionRepository(func(context.Context) ([]store.ScheduleProjectionInput, error) {
		return []store.ScheduleProjectionInput{{
			ScheduleID: "schedule-existing", ScheduleName: "Existing", ScheduleVersionID: "schedule-existing-v1", TaskID: "task-existing", TaskName: "Existing task", TaskVersionID: "task-existing-v1", Expression: "30 * * * *", Timezone: "UTC", RunnerPoolID: "pool-1", DurationSeconds: 1800,
			Resources: []store.ScheduleProjectionResource{{ID: "resource-1", Name: "Database", Kind: "exclusive"}},
		}}, nil
	}), nil)
	operations := NewOperationsService()
	operations.SetScheduleRepository(scheduleRepository)
	operations.SetTaskRepository(conflictTaskRepository{task: store.TaskRecord{ID: "task-candidate", CurrentVersionID: "task-candidate-v1", Name: "Candidate task", RunnerPoolID: "pool-1", DurationSeconds: 3600, ResourceIDs: []string{"resource-1"}}})
	operations.SetResourceRepository(conflictResourceRepository{})
	operations.SetScheduleProjection(service)
	response := httptest.NewRecorder()
	operations.scheduleCollection(response, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"task_id":"task-candidate","name":"Candidate","expression":"0 * * * *","timezone":"UTC"}`)))
	if response.Code != http.StatusConflict || scheduleRepository.created {
		t.Fatalf("schedule response=%d created=%v body=%s", response.Code, scheduleRepository.created, response.Body.String())
	}
	var body struct {
		Conflicts []controlplane.ProjectionConflict `json:"conflicts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conflicts) == 0 || body.Conflicts[0].ResourceName != "Database" || len(body.Conflicts[0].Occurrences) < 2 {
		t.Fatalf("conflict response = %#v", body)
	}
}

func TestTaskCreationRefreshesScheduleProjectionImmediately(t *testing.T) {
	calls := 0
	service := controlplane.NewProjectionService(conflictProjectionRepository(func(context.Context) ([]store.ScheduleProjectionInput, error) {
		calls++
		return nil, nil
	}), nil)
	operations := NewOperationsService()
	operations.SetScheduleProjection(service)
	response := httptest.NewRecorder()
	operations.taskCollection(response, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"name":"New task","command":["echo"],"runner_pool":"default"}`)))
	if response.Code != http.StatusCreated || calls != 1 || !service.Snapshot().Available {
		t.Fatalf("task response=%d refreshes=%d snapshot=%#v", response.Code, calls, service.Snapshot())
	}
}
