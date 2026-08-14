package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type RunRecord struct {
	ID           string `json:"id"`
	TaskID       string `json:"taskId"`
	TaskName     string `json:"taskName,omitempty"`
	State        string `json:"state"`
	Attempt      int    `json:"attempt"`
	Runner       string `json:"runner,omitempty"`
	Trigger      string `json:"trigger"`
	ScheduledFor string `json:"scheduledFor,omitempty"`
}

type LogChunk struct {
	Sequence int    `json:"sequence"`
	Text     string `json:"text"`
}

type RunService struct {
	mu   sync.RWMutex
	runs map[string]RunRecord
	logs map[string]map[string][]LogChunk
	next int
}

func NewRunService() *RunService {
	return &RunService{runs: map[string]RunRecord{}, logs: map[string]map[string][]LogChunk{}}
}

func (s *RunService) collection(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		items := make([]RunRecord, 0, len(s.runs))
		for _, run := range s.runs {
			items = append(items, run)
		}
		s.mu.RUnlock()
		sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
		writePage(w, r, items)
		return
	}
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}

func (s *RunService) execute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var input struct {
		TaskID string `json:"task_id"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || strings.TrimSpace(input.TaskID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task_id is required"})
		return
	}
	s.mu.Lock()
	s.next++
	run := RunRecord{ID: "run-" + strconv.Itoa(s.next), TaskID: input.TaskID, State: "WAITING", Attempt: 1, Trigger: "MANUAL"}
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
	run, ok := s.runs[id]
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	allowed := false
	switch action {
	case "cancel":
		allowed = run.State == "WAITING" || run.State == "RUNNING" || run.State == "RETRY_WAIT" || run.State == "CANCELLING"
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
