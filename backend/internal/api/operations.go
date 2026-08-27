package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type TaskRecord struct {
	ID                 string         `json:"id"`
	Name               string         `json:"name"`
	Enabled            bool           `json:"enabled"`
	IsDeleted          bool           `json:"isDeleted"`
	ActiveVersion      int            `json:"activeVersion"`
	Pool               string         `json:"pool"`
	PinnedRunner       string         `json:"pinnedRunner,omitempty"`
	Command            []string       `json:"command,omitempty"`
	WorkingDirectory   string         `json:"workingDirectory,omitempty"`
	PlacementSelectors map[string]any `json:"placementSelectors,omitempty"`
	Environment        map[string]any `json:"environment,omitempty"`
	SecretReferences   map[string]any `json:"secretReferences,omitempty"`
	DurationSeconds    int            `json:"durationSeconds"`
	MaxOutputBytes     int64          `json:"maxOutputBytes"`
	MaxAttempts        int            `json:"maxAttempts"`
	AmbiguityPolicy    string         `json:"ambiguityPolicy,omitempty"`
	Resources          []string       `json:"resources,omitempty"`
	LatestRun          *RunRecord     `json:"latestRun,omitempty"`
}

type TaskVersionRecord struct {
	ID                  string   `json:"id"`
	Version             int      `json:"version"`
	Pool                string   `json:"pool"`
	PinnedRunner        string   `json:"pinnedRunner,omitempty"`
	Command             []string `json:"command,omitempty"`
	WorkingDirectory    string   `json:"workingDirectory,omitempty"`
	DurationSeconds     int      `json:"durationSeconds"`
	MaxOutputBytes      int64    `json:"maxOutputBytes"`
	MaxAttempts         int      `json:"maxAttempts"`
	AmbiguityPolicy     string   `json:"ambiguityPolicy,omitempty"`
	Resources           []string `json:"resources,omitempty"`
	ExecutionSpecDigest string   `json:"executionSpecDigest,omitempty"`
	CreatedAt           string   `json:"createdAt,omitempty"`
}

type ScheduleRecord struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	TaskID            string `json:"taskId"`
	Enabled           bool   `json:"enabled"`
	NextFireAt        string `json:"nextFireAt,omitempty"`
	State             string `json:"state"`
	Timezone          string `json:"timezone"`
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
	resourceRepository         store.ResourceRepository
	scheduleProjection         *controlplane.ProjectionService
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

func (o *OperationsService) SetResourceRepository(repository store.ResourceRepository) {
	if repository != nil {
		o.mu.Lock()
		o.resourceRepository = repository
		o.mu.Unlock()
	}
}

func (o *OperationsService) SetScheduleProjection(projection *controlplane.ProjectionService) {
	o.mu.Lock()
	o.scheduleProjection = projection
	o.mu.Unlock()
}

func (o *OperationsService) hasDurableRepositories() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.repository != nil && o.scheduleRepository != nil
}

func taskRecordFromStore(task store.TaskRecord) TaskRecord {
	var latestRun *RunRecord
	if task.LatestRun != nil {
		mapped := runRecordFromStore(*task.LatestRun)
		latestRun = &mapped
	}
	return TaskRecord{ID: task.ID, Name: task.Name, Enabled: task.Enabled, IsDeleted: task.IsDeleted, ActiveVersion: task.ActiveVersion, Pool: task.RunnerPoolID, PinnedRunner: task.PinnedRunnerID, Command: append([]string(nil), task.Command...), WorkingDirectory: task.WorkingDirectory, PlacementSelectors: task.PlacementSelectors, Environment: task.Environment, SecretReferences: task.SecretReferences, DurationSeconds: task.DurationSeconds, MaxOutputBytes: task.MaxOutputBytes, MaxAttempts: task.MaxAttempts, AmbiguityPolicy: task.AmbiguityPolicy, Resources: append([]string(nil), task.ResourceIDs...), LatestRun: latestRun}
}

func taskVersionRecordFromStore(version store.TaskVersionRecord) TaskVersionRecord {
	createdAt := ""
	if !version.CreatedAt.IsZero() {
		createdAt = version.CreatedAt.UTC().Format(time.RFC3339)
	}
	return TaskVersionRecord{ID: version.ID, Version: version.Version, Pool: version.RunnerPoolID, PinnedRunner: version.PinnedRunnerID, Command: append([]string(nil), version.Command...), WorkingDirectory: version.WorkingDirectory, DurationSeconds: version.DurationSeconds, MaxOutputBytes: version.MaxOutputBytes, MaxAttempts: version.MaxAttempts, AmbiguityPolicy: version.AmbiguityPolicy, Resources: append([]string(nil), version.ResourceIDs...), ExecutionSpecDigest: version.ExecutionSpecDigest, CreatedAt: createdAt}
}

type taskInput struct {
	Name               string         `json:"name"`
	Command            []string       `json:"command"`
	RunnerPool         string         `json:"runner_pool"`
	PinnedRunner       string         `json:"pinned_runner"`
	WorkingDirectory   string         `json:"working_directory"`
	PlacementSelectors map[string]any `json:"placement_selectors"`
	Environment        map[string]any `json:"environment"`
	SecretReferences   map[string]any `json:"secret_references"`
	DurationSeconds    int            `json:"duration_seconds"`
	MaxOutputBytes     int64          `json:"max_output_bytes"`
	MaxAttempts        int            `json:"max_attempts"`
	AmbiguityPolicy    string         `json:"ambiguity_policy"`
	Resources          []string       `json:"resources"`
}

func taskDefinition(id string, input taskInput) store.TaskDefinition {
	return store.TaskDefinition{ID: id, Name: strings.TrimSpace(input.Name), RunnerPoolID: strings.TrimSpace(input.RunnerPool), PinnedRunnerID: strings.TrimSpace(input.PinnedRunner), Command: append([]string(nil), input.Command...), WorkingDirectory: input.WorkingDirectory, PlacementSelectors: input.PlacementSelectors, Environment: input.Environment, SecretReferences: input.SecretReferences, DurationSeconds: input.DurationSeconds, MaxOutputBytes: input.MaxOutputBytes, MaxAttempts: input.MaxAttempts, AmbiguityPolicy: input.AmbiguityPolicy, ResourceIDs: append([]string(nil), input.Resources...), Enabled: true}
}

func validateTaskSecrets(input taskInput) error {
	if len(input.SecretReferences) > 0 {
		return errors.New("task secret references are not supported")
	}
	return nil
}

func scheduleRecordFromStore(schedule store.ScheduleRecord) ScheduleRecord {
	nextFireAt := ""
	if schedule.NextFireAt != nil {
		nextFireAt = schedule.NextFireAt.UTC().Format(time.RFC3339)
	}
	return ScheduleRecord{ID: schedule.ID, Name: schedule.Name, TaskID: schedule.TaskID, Enabled: schedule.Enabled, NextFireAt: nextFireAt, State: schedule.State, Timezone: schedule.Timezone, Expression: schedule.Expression, MisfirePolicy: schedule.MisfirePolicy, CatchupLimit: schedule.CatchupLimit, DeadlineSeconds: schedule.DeadlineSeconds, ConcurrencyPolicy: schedule.ConcurrencyPolicy, MaxConcurrentRuns: schedule.MaxConcurrentRuns}
}

func scheduleDefinition(id string, input scheduleInput) (store.ScheduleDefinition, error) {
	definition := store.ScheduleDefinition{ID: id, Name: input.Name, TaskID: input.TaskID, Expression: input.Expression, Timezone: input.Timezone, Enabled: true, MisfirePolicy: input.MisfirePolicy, CatchupLimit: input.CatchupLimit, DeadlineSeconds: input.DeadlineSeconds, ConcurrencyPolicy: input.ConcurrencyPolicy, MaxConcurrentRuns: input.MaxConcurrentRuns}
	next, err := controlplane.NextFire(input.Expression, input.Timezone, time.Now().UTC())
	if err != nil {
		return store.ScheduleDefinition{}, err
	}
	definition.NextFireAt = &next
	return definition, nil
}

func (o *OperationsService) checkScheduleConflicts(ctx context.Context, definition store.ScheduleDefinition) ([]controlplane.ProjectionConflict, error) {
	o.mu.RLock()
	projection, taskRepository, resourceRepository := o.scheduleProjection, o.repository, o.resourceRepository
	o.mu.RUnlock()
	if projection == nil || taskRepository == nil || resourceRepository == nil {
		return nil, nil
	}
	task, found, err := taskRepository.Find(ctx, definition.TaskID)
	if err != nil {
		return nil, err
	}
	if !found || len(task.ResourceIDs) == 0 {
		return nil, nil
	}
	taskVersionID := task.CurrentVersionID
	if taskVersionID == "" {
		version := task.ActiveVersion
		if version < 1 {
			version = 1
		}
		taskVersionID = task.ID + "-v" + strconv.Itoa(version)
	}
	candidate := store.ScheduleProjectionInput{
		ScheduleID:        definition.ID,
		ScheduleName:      definition.Name,
		ScheduleVersionID: definition.ID + "-candidate",
		TaskID:            task.ID,
		TaskName:          task.Name,
		TaskVersionID:     taskVersionID,
		Expression:        definition.Expression,
		Timezone:          definition.Timezone,
		RunnerPoolID:      task.RunnerPoolID,
		PinnedRunnerID:    task.PinnedRunnerID,
		DurationSeconds:   task.DurationSeconds,
	}
	for _, resourceID := range task.ResourceIDs {
		resource, found, err := resourceRepository.Find(ctx, resourceID)
		if err != nil {
			return nil, err
		}
		if found {
			candidate.Resources = append(candidate.Resources, store.ScheduleProjectionResource{ID: resource.ID, Name: resource.Name, Kind: resource.Kind})
		}
	}
	if len(candidate.Resources) == 0 {
		return nil, nil
	}
	return projection.CheckScheduleConflicts(ctx, candidate, definition.ID)
}

func writeScheduleConflict(w http.ResponseWriter, conflicts []controlplane.ProjectionConflict) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error":     "schedule conflicts with exclusive resources",
		"code":      "exclusive_resource_conflict",
		"conflicts": conflicts,
	})
}

func (o *OperationsService) refreshScheduleProjection(ctx context.Context) {
	o.mu.RLock()
	projection := o.scheduleProjection
	o.mu.RUnlock()
	if projection != nil {
		_ = projection.Refresh(context.WithoutCancel(ctx))
	}
}

func (o *OperationsService) taskCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		archived := strings.EqualFold(r.URL.Query().Get("archived"), "true")
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			items, err := repository.List(r.Context(), archived)
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "task storage unavailable", err)
				return
			}
			result := make([]TaskRecord, 0, len(items))
			for _, item := range items {
				result = append(result, taskRecordFromStore(item))
			}
			result = filterTasks(result, r.URL.Query())
			writePage(w, r, result)
			return
		}
		o.mu.RLock()
		items := make([]TaskRecord, 0, len(o.tasks))
		for _, task := range o.tasks {
			if task.IsDeleted != archived {
				continue
			}
			items = append(items, task)
		}
		o.mu.RUnlock()
		items = filterTasks(items, r.URL.Query())
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		writePage(w, r, items)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input taskInput
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Name) == "" || len(input.Command) == 0 || strings.TrimSpace(input.RunnerPool) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task name, command, and runner pool are required"})
		return
	}
	if err := validateTaskSecrets(input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	o.mu.RLock()
	repository := o.repository
	o.mu.RUnlock()
	if repository != nil {
		id, err := randomID()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "task creation failed", err)
			return
		}
		created, err := repository.Create(r.Context(), taskDefinition("task-"+id, input))
		if err != nil {
			writeError(w, http.StatusBadRequest, "task creation failed", err)
			return
		}
		o.refreshScheduleProjection(r.Context())
		writeJSON(w, http.StatusCreated, taskRecordFromStore(created))
		return
	}
	task := o.createTask(input.Name, input.Command, input.RunnerPool, input.PinnedRunner, input.DurationSeconds, input.Resources)
	o.refreshScheduleProjection(r.Context())
	writeJSON(w, http.StatusCreated, task)
}

func (o *OperationsService) taskPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "tasks" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
		return
	}
	id := parts[3]
	if len(parts) == 5 && parts[4] == "versions" && r.Method == http.MethodGet {
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			versions, err := repository.ListVersions(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "task version storage unavailable", err)
				return
			}
			result := make([]TaskVersionRecord, 0, len(versions))
			for _, version := range versions {
				result = append(result, taskVersionRecordFromStore(version))
			}
			writeJSON(w, http.StatusOK, result)
			return
		}
		if _, ok := o.task(id); !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		writeJSON(w, http.StatusOK, []TaskVersionRecord{})
		return
	}
	if len(parts) == 4 && r.Method == http.MethodGet {
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			task, found, err := repository.Find(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "task storage unavailable", err)
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
	if len(parts) == 4 && r.Method == http.MethodDelete {
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			deleted, err := repository.Delete(r.Context(), id)
			if err != nil {
				recordRequestError(r, err)
				writeJSON(w, http.StatusConflict, map[string]string{"error": "task cannot be deleted"})
				return
			}
			if !deleted {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
				return
			}
			o.refreshScheduleProjection(r.Context())
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !o.deleteTask(id) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		o.refreshScheduleProjection(r.Context())
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 5 && parts[4] == "versions" && r.Method == http.MethodPost {
		var input taskInput
		if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Command) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
			return
		}
		if err := validateTaskSecrets(input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		o.mu.RLock()
		repository := o.repository
		o.mu.RUnlock()
		if repository != nil {
			updated, err := repository.CreateVersion(r.Context(), id, taskDefinition("", input))
			if err != nil {
				writeError(w, http.StatusBadRequest, "task version creation failed", err)
				return
			}
			o.refreshScheduleProjection(r.Context())
			writeJSON(w, http.StatusCreated, taskRecordFromStore(updated))
			return
		}
		updated, ok := o.addTaskVersion(id, input)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		o.refreshScheduleProjection(r.Context())
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
				writeError(w, http.StatusServiceUnavailable, "schedule storage unavailable", err)
				return
			}
			result := make([]ScheduleRecord, 0, len(items))
			for _, item := range items {
				result = append(result, scheduleRecordFromStore(item))
			}
			result = filterSchedules(result, r.URL.Query())
			writePage(w, r, result)
			return
		}
		o.mu.RLock()
		items := make([]ScheduleRecord, 0, len(o.schedules))
		for _, schedule := range o.schedules {
			items = append(items, schedule)
		}
		o.mu.RUnlock()
		items = filterSchedules(items, r.URL.Query())
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		writePage(w, r, items)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input scheduleInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule"})
		return
	}
	o.mu.RLock()
	repository := o.scheduleRepository
	o.mu.RUnlock()
	if repository != nil {
		id, err := randomID()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "schedule creation failed", err)
			return
		}
		definition, err := scheduleDefinition("schedule-"+id, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, "schedule creation failed", err)
			return
		}
		conflicts, err := o.checkScheduleConflicts(r.Context(), definition)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "schedule conflict check unavailable", err)
			return
		}
		if len(conflicts) > 0 {
			writeScheduleConflict(w, conflicts)
			return
		}
		created, err := repository.Create(r.Context(), definition)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		o.refreshScheduleProjection(r.Context())
		writeJSON(w, http.StatusCreated, scheduleRecordFromStore(created))
		return
	}
	schedule, err := o.saveSchedule(input, "")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	o.refreshScheduleProjection(r.Context())
	writeJSON(w, http.StatusCreated, schedule)
}

func (o *OperationsService) schedulePath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 4 {
		if len(parts) == 5 && (parts[4] == "enable" || parts[4] == "disable") && r.Method == http.MethodPost {
			id := parts[3]
			enabled := parts[4] == "enable"
			o.mu.RLock()
			repository := o.scheduleRepository
			o.mu.RUnlock()
			if repository != nil {
				item, found, err := repository.SetEnabled(r.Context(), id, enabled)
				if err != nil {
					writeError(w, http.StatusConflict, "schedule state update failed", err)
					return
				}
				if !found {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
					return
				}
				o.refreshScheduleProjection(r.Context())
				writeJSON(w, http.StatusOK, scheduleRecordFromStore(item))
				return
			}
			o.mu.Lock()
			item, found := o.schedules[id]
			if found {
				item.Enabled, item.State = enabled, map[bool]string{true: "ACTIVE", false: "DISABLED"}[enabled]
				o.schedules[id] = item
			}
			o.mu.Unlock()
			if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
				return
			}
			o.refreshScheduleProjection(r.Context())
			writeJSON(w, http.StatusOK, item)
			return
		}
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
				writeError(w, http.StatusServiceUnavailable, "schedule storage unavailable", err)
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
	if r.Method == http.MethodDelete {
		if repository != nil {
			deleted, err := repository.Delete(r.Context(), id)
			if err != nil {
				recordRequestError(r, err)
				writeJSON(w, http.StatusConflict, map[string]string{"error": "schedule cannot be deleted"})
				return
			}
			if !deleted {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
				return
			}
			o.refreshScheduleProjection(r.Context())
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if !o.deleteSchedule(id) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "schedule not found"})
			return
		}
		o.refreshScheduleProjection(r.Context())
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input scheduleInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid schedule"})
		return
	}
	if repository != nil {
		definition, err := scheduleDefinition(id, input)
		if err != nil {
			writeError(w, http.StatusBadRequest, "schedule update failed", err)
			return
		}
		conflicts, err := o.checkScheduleConflicts(r.Context(), definition)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "schedule conflict check unavailable", err)
			return
		}
		if len(conflicts) > 0 {
			writeScheduleConflict(w, conflicts)
			return
		}
		updated, err := repository.Update(r.Context(), id, definition)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		o.refreshScheduleProjection(r.Context())
		writeJSON(w, http.StatusOK, scheduleRecordFromStore(updated))
		return
	}
	schedule, err := o.saveSchedule(input, id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	o.refreshScheduleProjection(r.Context())
	writeJSON(w, http.StatusOK, schedule)
}

func (o *OperationsService) preview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input scheduleInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Expression == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "schedule expression is required"})
		return
	}
	occurrences, err := previewOccurrences(input.Expression, input.Timezone, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"occurrences": occurrences})
}

func previewOccurrences(expression, timezone string, now time.Time) ([]string, error) {
	cursor := now
	occurrences := make([]string, 0, 5)
	for range 5 {
		next, err := controlplane.NextFire(expression, timezone, cursor)
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, next.Format(time.RFC3339))
		cursor = next
	}
	return occurrences, nil
}

type scheduleInput struct {
	Name              string `json:"name"`
	TaskID            string `json:"task_id"`
	Expression        string `json:"expression"`
	Timezone          string `json:"timezone"`
	MisfirePolicy     string `json:"misfire_policy"`
	CatchupLimit      int    `json:"catchup_limit"`
	DeadlineSeconds   int    `json:"start_deadline_seconds"`
	ConcurrencyPolicy string `json:"concurrency_policy"`
	MaxConcurrentRuns int    `json:"max_concurrent_runs"`
}

func (o *OperationsService) createTask(name string, command []string, pool, pinnedRunner string, duration int, resources []string) TaskRecord {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.nextTaskID++
	task := TaskRecord{ID: "task-" + strconv.Itoa(o.nextTaskID), Name: strings.TrimSpace(name), Enabled: true, ActiveVersion: 1, Pool: strings.TrimSpace(pool), PinnedRunner: strings.TrimSpace(pinnedRunner), Command: append([]string(nil), command...), Resources: append([]string(nil), resources...), DurationSeconds: duration}
	o.tasks[task.ID] = task
	return task
}

func (o *OperationsService) task(id string) (TaskRecord, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	task, ok := o.tasks[id]
	if ok && task.IsDeleted {
		return TaskRecord{}, false
	}
	return task, ok
}

func (o *OperationsService) deleteTask(id string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	task, ok := o.tasks[id]
	if !ok || task.IsDeleted {
		return false
	}
	task.IsDeleted, task.Enabled = true, false
	o.tasks[id] = task
	for scheduleID, schedule := range o.schedules {
		if schedule.TaskID == id {
			delete(o.schedules, scheduleID)
		}
	}
	return true
}

func (o *OperationsService) addTaskVersion(id string, input taskInput) (TaskRecord, bool) {
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
	task.PinnedRunner = input.PinnedRunner
	task.Command = append([]string(nil), input.Command...)
	if input.Resources != nil {
		task.Resources = append([]string(nil), input.Resources...)
	}
	if input.DurationSeconds > 0 {
		task.DurationSeconds = input.DurationSeconds
	}
	task.ActiveVersion++
	o.tasks[id] = task
	return task, true
}

func (o *OperationsService) saveSchedule(input scheduleInput, id string) (ScheduleRecord, error) {
	if strings.TrimSpace(input.TaskID) == "" || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Expression) == "" || strings.TrimSpace(input.Timezone) == "" {
		return ScheduleRecord{}, errors.New("task, name, expression, and timezone are required")
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
	schedule := ScheduleRecord{ID: id, Name: input.Name, TaskID: input.TaskID, Enabled: true, State: "ACTIVE", Timezone: input.Timezone, Expression: input.Expression, MisfirePolicy: input.MisfirePolicy, CatchupLimit: input.CatchupLimit, DeadlineSeconds: input.DeadlineSeconds, ConcurrencyPolicy: input.ConcurrencyPolicy, MaxConcurrentRuns: input.MaxConcurrentRuns}
	o.schedules[id] = schedule
	return schedule, nil
}

func (o *OperationsService) schedule(id string) (ScheduleRecord, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	schedule, ok := o.schedules[id]
	return schedule, ok
}

func filterTasks(items []TaskRecord, query url.Values) []TaskRecord {
	search := strings.ToLower(strings.TrimSpace(query.Get("search")))
	state := strings.ToLower(strings.TrimSpace(query.Get("state")))
	if search == "" && state != "enabled" && state != "disabled" {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.ID), search) || strings.Contains(strings.ToLower(item.Name), search) || strings.Contains(strings.ToLower(item.Pool), search) {
			if state == "enabled" && !item.Enabled || state == "disabled" && item.Enabled {
				continue
			}
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func filterSchedules(items []ScheduleRecord, query url.Values) []ScheduleRecord {
	task, due := strings.TrimSpace(query.Get("task")), strings.EqualFold(query.Get("due"), "true")
	enabled, enabledFilter := false, false
	if value := strings.TrimSpace(query.Get("enabled")); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			enabled, enabledFilter = parsed, true
		}
	}
	if task == "" && !due && !enabledFilter {
		return items
	}
	filtered := items[:0]
	now := time.Now().UTC()
	for _, item := range items {
		if task != "" && !strings.Contains(strings.ToLower(item.TaskID), strings.ToLower(task)) {
			continue
		}
		if enabledFilter && item.Enabled != enabled {
			continue
		}
		if due {
			at, err := time.Parse(time.RFC3339, item.NextFireAt)
			if err != nil || at.After(now) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func (o *OperationsService) deleteSchedule(id string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.schedules[id]; !ok {
		return false
	}
	delete(o.schedules, id)
	return true
}

const maxCollectionPageLimit = 100

func writePage[T any](w http.ResponseWriter, r *http.Request, items []T) {
	all := strings.EqualFold(r.URL.Query().Get("all"), "true")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if all {
		page = 1
		limit = maxCollectionPageLimit
	} else if limit < 1 || limit > maxCollectionPageLimit {
		limit = 50
	}
	start := 0
	if page > 1 {
		if page-1 > len(items)/limit {
			start = len(items)
		} else {
			start = (page - 1) * limit
		}
	}
	end := len(items)
	if limit <= len(items)-start {
		end = start + limit
	}
	pages := len(items) / limit
	if len(items)%limit != 0 {
		pages++
	}
	if pages == 0 {
		pages = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items[start:end], "page": page, "limit": limit, "total": len(items), "pages": pages})
}
