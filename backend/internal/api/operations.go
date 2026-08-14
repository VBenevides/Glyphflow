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
}

func NewOperationsService() *OperationsService {
	return &OperationsService{tasks: map[string]TaskRecord{}, schedules: map[string]ScheduleRecord{}}
}

func (o *OperationsService) taskCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
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
	if r.Method == http.MethodGet {
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
