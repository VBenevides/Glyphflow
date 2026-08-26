package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type RunRecord struct {
	ID                string `json:"id"`
	TaskID            string `json:"taskId"`
	TaskName          string `json:"taskName,omitempty"`
	State             string `json:"state"`
	PlacementBlocker  string `json:"placementBlocker,omitempty"`
	Attempt           int    `json:"attempt"`
	ExitCode          *int   `json:"exitCode,omitempty"`
	ExitCodeMeaning   string `json:"exitCodeMeaning"`
	Error             string `json:"error,omitempty"`
	Runner            string `json:"runner,omitempty"`
	Trigger           string `json:"trigger"`
	ScheduledFor      string `json:"scheduledFor,omitempty"`
	MaxMemoryUsed     int64  `json:"maxMemoryUsedBytes"`
	AverageMemoryUsed int64  `json:"averageMemoryUsedBytes"`
}

type LogChunk struct {
	Sequence int    `json:"sequence"`
	Text     string `json:"text"`
}

type RunService struct {
	mu         sync.RWMutex
	runs       map[string]RunRecord
	logs       map[string]map[string][]LogChunk
	repository store.RunRepository
}

func NewRunService() *RunService {
	return &RunService{runs: map[string]RunRecord{}, logs: map[string]map[string][]LogChunk{}}
}

func (s *RunService) SetRepository(repository store.RunRepository) {
	if repository != nil {
		s.mu.Lock()
		s.repository = repository
		s.mu.Unlock()
	}
}

func (s *RunService) hasDurableRepository() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.repository != nil
}

func runRecordFromStore(run store.RunRecord) RunRecord {
	scheduledFor := ""
	if !run.ScheduledFor.IsZero() {
		scheduledFor = run.ScheduledFor.UTC().Format(time.RFC3339)
	}
	return RunRecord{ID: run.ID, TaskID: run.TaskID, TaskName: run.TaskName, State: run.State, PlacementBlocker: run.PlacementBlocker, Attempt: run.Attempt, ExitCode: run.ExitCode, ExitCodeMeaning: run.ExitCodeMeaning, Error: run.Error, Runner: run.Runner, Trigger: run.TriggerType, ScheduledFor: scheduledFor, MaxMemoryUsed: run.MaxMemoryUsedBytes, AverageMemoryUsed: run.AverageMemoryUsedBytes}
}

func (s *RunService) collection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		filter, page, limit, valid := runListFilterChecked(r)
		if !valid {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": paginationOffsetError})
			return
		}
		s.mu.RLock()
		repository := s.repository
		s.mu.RUnlock()
		if repository != nil {
			if paged, ok := repository.(store.RunPageRepository); ok {
				result, err := paged.ListPage(r.Context(), filter)
				if err != nil {
					writeError(w, http.StatusServiceUnavailable, "run storage unavailable", err)
					return
				}
				items := make([]RunRecord, 0, len(result.Items))
				for _, item := range result.Items {
					items = append(items, runRecordFromStore(item))
				}
				writeRunPage(w, page, limit, result.Total, items)
				return
			}
			items, err := repository.List(r.Context())
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "run storage unavailable", err)
				return
			}
			result := make([]RunRecord, 0, len(items))
			for _, item := range items {
				result = append(result, runRecordFromStore(item))
			}
			result = filterRuns(result, r.URL.Query())
			writePage(w, r, result)
			return
		}
		s.mu.RLock()
		items := make([]RunRecord, 0, len(s.runs))
		for _, run := range s.runs {
			items = append(items, run)
		}
		s.mu.RUnlock()
		items = filterRuns(items, r.URL.Query())
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		writePage(w, r, items)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func runListFilter(r *http.Request) (store.RunListFilter, int, int) {
	filter, page, limit, _ := runListFilterChecked(r)
	return filter, page, limit
}

const paginationOffsetError = "pagination offset exceeds safe integer range"

func checkedPaginationOffset(page, limit int) (int, bool) {
	if page < 1 || limit < 1 {
		return 0, false
	}
	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/limit {
		return 0, false
	}
	return (page - 1) * limit, true
}

func runListFilterChecked(r *http.Request) (store.RunListFilter, int, int, bool) {
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(query.Get("limit"))
	all := strings.EqualFold(query.Get("all"), "true")
	if all {
		page = 1
		limit = 100
	} else if limit < 1 || limit > 100 {
		limit = 50
	}
	from, _ := parseFilterTime(query.Get("from"))
	to, _ := parseFilterTime(query.Get("to"))
	offset, valid := checkedPaginationOffset(page, limit)
	return store.RunListFilter{State: query.Get("state"), Task: query.Get("task"), Runner: query.Get("runner"), Trigger: query.Get("trigger"), From: from, To: to, Limit: limit, Offset: offset}, page, limit, valid
}

func writeRunPage(w http.ResponseWriter, page, limit, total int, items []RunRecord) {
	pages := total / limit
	if total%limit != 0 {
		pages++
	}
	if pages == 0 {
		pages = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "page": page, "limit": limit, "total": total, "pages": pages})
}

func filterRuns(items []RunRecord, query url.Values) []RunRecord {
	state := strings.ToUpper(strings.TrimSpace(query.Get("state")))
	task, runner, trigger := strings.ToLower(strings.TrimSpace(query.Get("task"))), strings.ToLower(strings.TrimSpace(query.Get("runner"))), strings.ToUpper(strings.TrimSpace(query.Get("trigger")))
	from, _ := parseFilterTime(query.Get("from"))
	to, _ := parseFilterTime(query.Get("to"))
	if state == "" && task == "" && runner == "" && trigger == "" && from.IsZero() && to.IsZero() {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		match := item.State == state
		if state == "ACTIVE" {
			match = isActiveRunState(item.State)
		}
		if state == "" {
			match = true
		}
		if task != "" {
			match = match && (strings.Contains(strings.ToLower(item.TaskID), task) || strings.Contains(strings.ToLower(item.TaskName), task))
		}
		if runner != "" {
			match = match && strings.EqualFold(item.Runner, runner)
		}
		if trigger != "" {
			match = match && strings.EqualFold(item.Trigger, trigger)
		}
		if at, err := parseFilterTime(item.ScheduledFor); !from.IsZero() {
			match = match && err == nil && !at.Before(from)
		}
		if at, err := parseFilterTime(item.ScheduledFor); !to.IsZero() {
			match = match && err == nil && !at.After(to)
		}
		if match {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func parseFilterTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		parsed, err = time.Parse("2006-01-02T15:04", value)
	}
	return parsed, err
}

func isActiveRunState(state string) bool {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "WAITING", "DISPATCHED", "RUNNING", "RETRY_WAIT", "CANCELLING":
		return true
	default:
		return false
	}
}

func (s *RunService) execute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		TaskID         string `json:"task_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.TaskID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task_id is required"})
		return
	}
	s.mu.RLock()
	repository := s.repository
	s.mu.RUnlock()
	if repository != nil {
		id, err := store.NewRunID()
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "run creation failed", err)
			return
		}
		idempotencyKey := input.IdempotencyKey
		if idempotencyKey == "" {
			idempotencyKey = r.Header.Get("Idempotency-Key")
		}
		created, err := repository.Create(r.Context(), store.RunDefinition{ID: id, TaskID: input.TaskID, TriggerType: "MANUAL", IdempotencyKey: idempotencyKey, ScheduledFor: time.Now().UTC()})
		if err != nil {
			if errors.Is(err, store.ErrStorageExhausted) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage_exhausted"})
				return
			}
			if errors.Is(err, store.ErrStorageUnavailable) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "storage_unavailable"})
				return
			}
			writeError(w, http.StatusConflict, "run creation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, runRecordFromStore(created))
		return
	}
	id, err := store.NewRunID()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "run creation failed", err)
		return
	}
	s.mu.Lock()
	run := RunRecord{ID: id, TaskID: input.TaskID, State: "WAITING", Attempt: 1, Trigger: "MANUAL"}
	s.runs[run.ID] = run
	s.logs[run.ID] = map[string][]LogChunk{"stdout": {}, "stderr": {}}
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, run)
}

func (s *RunService) path(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	id := parts[3]
	if len(parts) == 4 && r.Method == http.MethodGet {
		s.mu.RLock()
		repository := s.repository
		s.mu.RUnlock()
		if repository != nil {
			run, found, err := repository.Find(r.Context(), id)
			if err != nil {
				writeError(w, http.StatusServiceUnavailable, "run storage unavailable", err)
			} else if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			} else {
				writeJSON(w, http.StatusOK, runRecordFromStore(run))
			}
			return
		}
		s.mu.RLock()
		run, ok := s.runs[id]
		s.mu.RUnlock()
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			return
		}
		writeJSON(w, http.StatusOK, run)
		return
	}
	if len(parts) >= 5 && parts[4] == "logs" {
		s.logsResponse(w, r, id, false)
		return
	}
	if len(parts) == 5 && (parts[4] == "cancel" || parts[4] == "retry" || parts[4] == "reconcile") {
		s.action(w, r, id, parts[4])
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "run route not found"})
}

func (s *RunService) logsResponse(w http.ResponseWriter, r *http.Request, id string, _ bool) {
	s.mu.RLock()
	repository := s.repository
	s.mu.RUnlock()
	if repository != nil {
		stream := r.URL.Query().Get("stream")
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		chunks, err := repository.ListLogChunks(r.Context(), id, stream, after)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "log storage unavailable", err)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			for _, chunk := range chunks {
				_, _ = w.Write([]byte(chunk.Text))
			}
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, chunk := range chunks {
			_ = json.NewEncoder(w).Encode(LogChunk{Sequence: int(chunk.Sequence), Text: chunk.Text})
		}
		return
	}
	s.mu.RLock()
	streams := s.logs[id]
	chunks := append([]LogChunk(nil), streams[r.URL.Query().Get("stream")]...)
	_, exists := s.runs[id]
	s.mu.RUnlock()
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	if strings.HasSuffix(r.URL.Path, "/download") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, chunk := range chunks {
			_, _ = w.Write([]byte(chunk.Text))
		}
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	for _, chunk := range chunks {
		if chunk.Sequence > after {
			_ = json.NewEncoder(w).Encode(chunk)
		}
	}
}

func (s *RunService) action(w http.ResponseWriter, r *http.Request, id, action string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason is required"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.repository != nil {
		if action == "cancel" {
			if cancellation, ok := s.repository.(store.CancellationRepository); ok {
				updated, changed, err := cancellation.RequestCancellation(r.Context(), id, input.Reason)
				if err != nil {
					writeError(w, http.StatusServiceUnavailable, "run cancellation failed", err)
					return
				}
				if !changed {
					if _, found, findErr := s.repository.Find(r.Context(), id); findErr != nil || !found {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
					} else {
						writeJSON(w, http.StatusConflict, map[string]string{"error": "run action is not allowed in the current state"})
					}
					return
				}
				writeJSON(w, http.StatusOK, runRecordFromStore(updated))
				return
			}
		}
		if action == "retry" || action == "reconcile" {
			if retryRepository, ok := s.repository.(store.RetryRepository); ok {
				updated, changed, err := retryRepository.Retry(r.Context(), id, input.Reason)
				if err != nil {
					writeError(w, http.StatusServiceUnavailable, "run retry failed", err)
					return
				}
				if !changed {
					if _, found, findErr := s.repository.Find(r.Context(), id); findErr != nil || !found {
						writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
					} else {
						writeJSON(w, http.StatusConflict, map[string]string{"error": "run retry is not allowed in the current state"})
					}
					return
				}
				writeJSON(w, http.StatusOK, runRecordFromStore(updated))
				return
			}
		}
		var from []string
		var to string
		switch action {
		case "cancel":
			from, to = []string{"WAITING", "DISPATCHED", "RUNNING", "RETRY_WAIT", "CANCELLING"}, "CANCELLED"
		case "retry":
			from, to = []string{"FAILED"}, "RETRY_WAIT"
		case "reconcile":
			from, to = []string{"UNKNOWN"}, "RETRY_WAIT"
		}
		updated, changed, err := s.repository.Transition(r.Context(), id, from, to)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "run transition failed", err)
			return
		}
		if !changed {
			if _, found, findErr := s.repository.Find(r.Context(), id); findErr != nil || !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
			} else {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "run action is not allowed in the current state"})
			}
			return
		}
		writeJSON(w, http.StatusOK, runRecordFromStore(updated))
		return
	}
	run, ok := s.runs[id]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	allowed := false
	switch action {
	case "cancel":
		allowed = run.State == "WAITING" || run.State == "DISPATCHED" || run.State == "RUNNING" || run.State == "RETRY_WAIT" || run.State == "CANCELLING"
		if allowed {
			run.State = "CANCELLED"
		}
	case "retry":
		allowed = run.State == "FAILED" || run.State == "TIMED_OUT"
		if allowed {
			run.State, run.Attempt = "RETRY_WAIT", run.Attempt+1
		}
	case "reconcile":
		allowed = run.State == "UNKNOWN"
		if allowed {
			run.State = "RETRY_WAIT"
		}
	}
	if !allowed {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "run action is not allowed in the current state"})
		return
	}
	s.runs[id] = run
	writeJSON(w, http.StatusOK, run)
}
