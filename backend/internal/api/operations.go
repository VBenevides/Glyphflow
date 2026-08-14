package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type TaskRecord struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	ActiveVersion  int      `json:"activeVersion"`
	Pool           string   `json:"pool"`
	Command        []string `json:"command,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

type ScheduleRecord struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	TaskID            string `json:"taskId"`
	Enabled           bool   `json:"enabled"`
	NextFireAt        string `json:"nextFireAt,omitempty"`
	State             string `json:"state"`
	Timezone          string `json:"timezone"`
	ScheduleType      string `json:"scheduleType"`
	Expression        string `json:"expression"`
	MisfirePolicy     string `json:"misfirePolicy"`
	CatchupLimit      int    `json:"catchupLimit"`
	DeadlineSeconds   int    `json:"deadlineSeconds"`
	ConcurrencyPolicy string `json:"concurrencyPolicy"`
	MaxConcurrentRuns int    `json:"maxConcurrentRuns"`
}

type OperationsService struct {
	mu                         sync.RWMutex
	tasks                      map[string]TaskRecord
	schedules                  map[string]ScheduleRecord
	nextTaskID, nextScheduleID int
	repository                 store.TaskRepository
	scheduleRepository         store.ScheduleRepository
}

func NewOperationsService() *OperationsService {
	return &OperationsService{tasks: map[string]TaskRecord{}, schedules: map[string]ScheduleRecord{}}
}

func (o *OperationsService) SetTaskRepository(repository store.TaskRepository) {
	if repository != nil {
		o.mu.Lock()
		o.repository = repository
		o.mu.Unlock()
	}
}

func (o *OperationsService) SetScheduleRepository(repository store.ScheduleRepository) {
	if repository != nil {
		o.mu.Lock()
		o.scheduleRepository = repository
		o.mu.Unlock()
	}
}

func taskRecordFromStore(task store.TaskRecord) TaskRecord {
	return TaskRecord{ID: task.ID, Name: task.Name, Enabled: task.Enabled, ActiveVersion: task.ActiveVersion, Pool: task.RunnerPoolID, Command: append([]string(nil), task.Command...), TimeoutSeconds: task.TimeoutSeconds}
}

func taskDefinition(id, name, pool string, command []string, timeout int) store.TaskDefinition {
	return store.TaskDefinition{ID: id, Name: strings.TrimSpace(name), RunnerPoolID: strings.TrimSpace(pool), Command: append([]string(nil), command...), TimeoutSeconds: timeout, Enabled: true}
}

func scheduleRecordFromStore(schedule store.ScheduleRecord) ScheduleRecord {
	nextFireAt := ""
	if schedule.NextFireAt != nil {
		nextFireAt = schedule.NextFireAt.UTC().Format(time.RFC3339)
	}
	return ScheduleRecord{ID: schedule.ID, Name: schedule.Name, TaskID: schedule.TaskID, Enabled: schedule.Enabled, NextFireAt: nextFireAt, State: schedule.State, Timezone: schedule.Timezone, ScheduleType: schedule.ScheduleType, Expression: schedule.Expression, MisfirePolicy: schedule.MisfirePolicy, CatchupLimit: schedule.CatchupLimit, DeadlineSeconds: schedule.DeadlineSeconds, ConcurrencyPolicy: schedule.ConcurrencyPolicy, MaxConcurrentRuns: schedule.MaxConcurrentRuns}
}

func scheduleDefinition(id string, input scheduleInput) store.ScheduleDefinition {
	return store.ScheduleDefinition{ID: id, Name: input.Name, TaskID: input.TaskID, ScheduleType: input.ScheduleType, Expression: input.Expression, Timezone: input.Timezone, Enabled: true, MisfirePolicy: input.MisfirePolicy, CatchupLimit: input.CatchupLimit, DeadlineSeconds: input.DeadlineSeconds, ConcurrencyPolicy: input.ConcurrencyPolicy, MaxConcurrentRuns: input.MaxConcurrentRuns}
}

func (o *OperationsService) taskCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			items, err := repository.List(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task storage unavailable"})
				return
			}
			result := make([]TaskRecord, 0, len(items))
			for _, item := range items {
				result = append(result, taskRecordFromStore(item))
			}
			writePage(w, r, result)
			return
		}
		o.mu.RLock()
		items := make([]TaskRecord, 0, len(o.tasks))
		for _, task := range o.tasks {
			items = append(items, task)
		}
		o.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		writePage(w, r, items)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Name           string   `json:"name"`
		Command        []string `json:"command"`
		RunnerPool     string   `json:"runner_pool"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" || len(input.Command) == 0 || strings.TrimSpace(input.RunnerPool) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task name, command, and runner pool are required"})
		return
	}
	o.mu.RLock()
	repository := o.repository
	o.mu.RUnlock()
	if repository != nil {
		id, err := randomID()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task creation failed"})
			return
		}
		created, err := repository.Create(r.Context(), taskDefinition("task-"+id, input.Name, input.RunnerPool, input.Command, input.TimeoutSeconds))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task creation failed"})
			return
		}
		writeJSON(w, http.StatusCreated, taskRecordFromStore(created))
		return
	}
	task := o.createTask(input.Name, input.Command, input.RunnerPool, input.TimeoutSeconds)
	writeJSON(w, http.StatusCreated, task)
}

func (o *OperationsService) taskPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "tasks" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	id := parts[3]
	if len(parts) == 4 && r.Method == http.MethodGet {
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			task, found, err := repository.Find(r.Context(), id)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "task storage unavailable"})
			} else if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			} else {
				writeJSON(w, http.StatusOK, taskRecordFromStore(task))
			}
			return
		}
		if task, ok := o.task(id); ok {
			writeJSON(w, http.StatusOK, task)
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		}
		return
	}
	if len(parts) == 5 && parts[4] == "versions" && r.Method == http.MethodPost {
		var input struct {
			Name           string   `json:"name"`
			Command        []string `json:"command"`
			RunnerPool     string   `json:"runner_pool"`
			TimeoutSeconds int      `json:"timeout_seconds"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Command) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
			return
		}
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			updated, err := repository.CreateVersion(r.Context(), id, taskDefinition("", input.Name, input.RunnerPool, input.Command, input.TimeoutSeconds))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task version creation failed"})
				return
			}
			writeJSON(w, http.StatusCreated, taskRecordFromStore(updated))
			return
		}
		updated, ok := o.addTaskVersion(id, input)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		writeJSON(w, http.StatusCreated, updated)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "task route not found"})
}

func (o *OperationsService) scheduleCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		o.mu.RLock()
		repository := o.scheduleRepository
		o.mu.RUnlock()
		if repository != nil {
			items, err := repository.List(r.Context())
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedule storage unavailable"})
				return
			}
			result := make([]ScheduleRecord, 0, len(items))
			for _, item := range items {
				result = append(result, scheduleRecordFromStore(item))
			}
			writePage(w, r, result)
			return
		}
		o.mu.RLock()
		items := make([]ScheduleRecord, 0, len(o.schedules))
		for _, schedule := range o.schedules {
			items = append(items, schedule)
		}
		o.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		writePage(w, r, items)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input scheduleInput
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule"})
		return
	}
	o.mu.RLock()
	repository := o.scheduleRepository
	o.mu.RUnlock()
	if repository != nil {
		id, err := randomID()
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedule creation failed"})
			return
		}
		created, err := repository.Create(r.Context(), scheduleDefinition("schedule-"+id, input))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, scheduleRecordFromStore(created))
		return
	}
	schedule, err := o.saveSchedule(input, "")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, schedule)
}

func (o *OperationsService) schedulePath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		return
	}
	id := parts[3]
	o.mu.RLock()
	repository := o.scheduleRepository
	o.mu.RUnlock()
	if r.Method == http.MethodGet {
		if repository != nil {
			schedule, found, err := repository.Find(r.Context(), id)
			if err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "schedule storage unavailable"})
			} else if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
			} else {
				writeJSON(w, http.StatusOK, scheduleRecordFromStore(schedule))
			}
			return
		}
		if schedule, ok := o.schedule(id); ok {
			writeJSON(w, http.StatusOK, schedule)
		} else {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
		}
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input scheduleInput
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule"})
		return
	}
	if repository != nil {
		updated, err := repository.Update(r.Context(), id, scheduleDefinition(id, input))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, scheduleRecordFromStore(updated))
		return
	}
	schedule, err := o.saveSchedule(input, id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, schedule)
}

func (o *OperationsService) preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input scheduleInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.Expression == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule expression is required"})
		return
	}
	next := time.Now().UTC().Add(time.Minute)
	if input.ScheduleType == "cron" {
		value, err := (controlplane.Schedule{Cron: input.Expression, Timezone: input.Timezone}).Next(time.Now())
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		next = value
	}
	writeJSON(w, http.StatusOK, map[string][]string{"occurrences": {next.Format(time.RFC3339)}})
}

type scheduleInput struct {
	Name              string `json:"name"`
	TaskID            string `json:"task_id"`
	ScheduleType      string `json:"schedule_type"`
	Expression        string `json:"expression"`
	Timezone          string `json:"timezone"`
	MisfirePolicy     string `json:"misfire_policy"`
	CatchupLimit      int    `json:"catchup_limit"`
	DeadlineSeconds   int    `json:"start_deadline_seconds"`
	ConcurrencyPolicy string `json:"concurrency_policy"`
	MaxConcurrentRuns int    `json:"max_concurrent_runs"`
}

func (o *OperationsService) createTask(name string, command []string, pool string, timeout int) TaskRecord {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.nextTaskID++
	task := TaskRecord{ID: "task-" + strconv.Itoa(o.nextTaskID), Name: strings.TrimSpace(name), Enabled: true, ActiveVersion: 1, Pool: strings.TrimSpace(pool), Command: append([]string(nil), command...), TimeoutSeconds: timeout}
	o.tasks[task.ID] = task
	return task
}

func (o *OperationsService) task(id string) (TaskRecord, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	task, ok := o.tasks[id]
	return task, ok
}

func (o *OperationsService) addTaskVersion(id string, input struct {
	Name           string   `json:"name"`
	Command        []string `json:"command"`
	RunnerPool     string   `json:"runner_pool"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}) (TaskRecord, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	task, ok := o.tasks[id]
	if !ok {
		return TaskRecord{}, false
	}
	if input.Name != "" {
		task.Name = input.Name
	}
	if input.RunnerPool != "" {
		task.Pool = input.RunnerPool
	}
	task.Command = append([]string(nil), input.Command...)
	if input.TimeoutSeconds > 0 {
		task.TimeoutSeconds = input.TimeoutSeconds
	}
	task.ActiveVersion++
	o.tasks[id] = task
	return task, true
}

func (o *OperationsService) saveSchedule(input scheduleInput, id string) (ScheduleRecord, error) {
	if strings.TrimSpace(input.TaskID) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Expression) == "" || strings.TrimSpace(input.Timezone) == "" {
		return ScheduleRecord{}, errors.New("task, name, expression, and timezone are required")
	}
	if input.ScheduleType == "" {
		input.ScheduleType = "cron"
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if id == "" {
		o.nextScheduleID++
		id = "schedule-" + strconv.Itoa(o.nextScheduleID)
	}
	if existing, ok := o.schedules[id]; ok && input.Name == "" {
		input.Name = existing.Name
	}
	schedule := ScheduleRecord{ID: id, Name: input.Name, TaskID: input.TaskID, Enabled: true, State: "ACTIVE", Timezone: input.Timezone, ScheduleType: input.ScheduleType, Expression: input.Expression, MisfirePolicy: input.MisfirePolicy, CatchupLimit: input.CatchupLimit, DeadlineSeconds: input.DeadlineSeconds, ConcurrencyPolicy: input.ConcurrencyPolicy, MaxConcurrentRuns: input.MaxConcurrentRuns}
	o.schedules[id] = schedule
	return schedule, nil
}

func (o *OperationsService) schedule(id string) (ScheduleRecord, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	schedule, ok := o.schedules[id]
	return schedule, ok
}

func writePage[T any](w http.ResponseWriter, r *http.Request, items []T) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 50
	}
	start := (page - 1) * limit
	if start > len(items) {
		start = len(items)
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	pages := (len(items) + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items[start:end], "page": page, "limit": limit, "total": len(items), "pages": pages})
}
