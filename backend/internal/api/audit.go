package api

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type AuditEvent struct {
	ID            string         `json:"id"`
	Actor         string         `json:"actor"`
	ActorName     string         `json:"actorName,omitempty"`
	ActorEmail    string         `json:"actorEmail,omitempty"`
	Action        string         `json:"action"`
	Description   string         `json:"description,omitempty"`
	Target        string         `json:"target"`
	Result        string         `json:"result"`
	Request       string         `json:"request,omitempty"`
	Input         any            `json:"input,omitempty"`
	Output        any            `json:"output,omitempty"`
	Traceback     string         `json:"traceback,omitempty"`
	CorrelationID string         `json:"correlationId,omitempty"`
	CreatedAt     string         `json:"createdAt"`
	Before        map[string]any `json:"before,omitempty"`
	After         map[string]any `json:"after,omitempty"`
}

func auditDescription(method, path string) string {
	switch {
	case path == "/api/v1/admin/auth/settings":
		return "Update authentication settings"
	case path == "/api/v1/admin/auth/sessions/revoke":
		return "Revoke user session"
	case path == "/api/v1/admin/auth/providers":
		if method == http.MethodGet {
			return "List SSO providers"
		}
		return "Update SSO provider"
	case strings.HasPrefix(path, "/api/v1/admin/auth/users/"):
		return "Disable user"
	case path == "/api/v1/admin/roles":
		if method == http.MethodGet {
			return "List roles"
		}
		return "Create role"
	case strings.HasPrefix(path, "/api/v1/admin/roles/"):
		if method == http.MethodDelete {
			return "Delete role"
		}
		return "Update role"
	case path == "/api/v1/roles":
		if method == http.MethodGet {
			return "List roles"
		}
		return "Manage roles"
	case path == "/api/v1/sso":
		if method == http.MethodGet {
			return "View SSO configuration"
		}
		return "Update SSO configuration"
	case path == "/api/v1/logs":
		return "List logs"
	case path == "/api/v1/events":
		return "List events"
	case path == "/api/v1/users":
		if method == http.MethodGet {
			return "List users"
		}
		return "Create user"
	case strings.HasPrefix(path, "/api/v1/users/"):
		return "View user details"
	case path == "/api/v1/audit":
		return "View audit events"
	case path == "/api/v1/me":
		if method == http.MethodGet {
			return "View own profile"
		}
		return "Update own profile"
	case path == "/api/v1/me/password":
		return "Change own password"
	case strings.HasPrefix(path, "/api/v1/me/"):
		return "Manage own account"
	case path == "/api/v1/tasks":
		if method == http.MethodGet {
			return "List tasks"
		}
		return "Create task"
	case path == "/api/v1/schedules":
		if method == http.MethodGet {
			return "List schedules"
		}
		return "Create schedule"
	case path == "/api/v1/schedules/preview":
		return "Preview schedule occurrences"
	case strings.HasPrefix(path, "/api/v1/schedules/"):
		if method == http.MethodDelete {
			return "Delete schedule"
		}
		if method == http.MethodGet {
			return "View schedule"
		}
		return "Update schedule"
	case strings.HasPrefix(path, "/api/v1/tasks/"):
		if strings.HasSuffix(path, "/versions") {
			return "Publish task version"
		}
		if strings.HasSuffix(path, "/cancel") {
			return "Cancel task run"
		}
		if strings.HasSuffix(path, "/retry") {
			return "Retry task run"
		}
		if method == http.MethodDelete {
			return "Delete task"
		}
		if method == http.MethodGet {
			return "View task"
		}
		return "Update task"
	case path == "/api/v1/resources":
		if method == http.MethodGet {
			return "List resources"
		}
		return "Create resource"
	case strings.HasPrefix(path, "/api/v1/resources/"):
		if strings.HasSuffix(path, "/lease") {
			if method == http.MethodPost {
				return "Acquire resource lease"
			}
			return "Release resource lease"
		}
		if method == http.MethodGet {
			return "View resource"
		}
		if method == http.MethodDelete {
			return "Delete resource"
		}
		return "Update resource"
	case path == "/api/v1/runners":
		return "List runners"
	case strings.HasPrefix(path, "/api/v1/runners/"):
		if strings.HasSuffix(path, "/enrollments") {
			return "Create runner enrollment"
		}
		if method == http.MethodDelete {
			return "Delete runner"
		}
		for action, description := range map[string]string{"enable": "Enable runner", "disable": "Disable runner", "drain": "Drain runner", "reset": "Reset runner", "revoke": "Revoke runner"} {
			if strings.HasSuffix(path, "/"+action) {
				return description
			}
		}
		if method == http.MethodGet {
			return "View runner"
		}
		return "Update runner"
	case path == "/api/v1/runs":
		return "List runs"
	case path == "/api/v1/runs/execute":
		return "Start task run"
	case path == "/api/v1/runs/retry":
		return "Retry run"
	case path == "/api/v1/runs/cancel":
		return "Cancel run"
	case strings.HasPrefix(path, "/api/v1/runs/"):
		if strings.HasSuffix(path, "/logs/download") {
			return "Download run logs"
		}
		if strings.HasSuffix(path, "/logs") {
			return "Stream run logs"
		}
		if strings.HasSuffix(path, "/events") {
			return "List run events"
		}
		if strings.HasSuffix(path, "/cancel") {
			return "Cancel run"
		}
		if strings.HasSuffix(path, "/retry") {
			return "Retry run"
		}
		if strings.HasSuffix(path, "/reconcile") {
			return "Reconcile run"
		}
		if method == http.MethodGet {
			return "View run"
		}
		return "Manage run"
	default:
		return strings.TrimSpace(method + " " + path)
	}
}

type AuditQueryService struct {
	mu         sync.RWMutex
	events     []AuditEvent
	repository store.AuditRepository
}

func NewAuditQueryService() *AuditQueryService { return &AuditQueryService{} }

func (s *AuditQueryService) SetRepository(repository store.AuditRepository) {
	if repository != nil {
		s.mu.Lock()
		s.repository = repository
		s.mu.Unlock()
	}
}

func auditEventFromStore(event store.AuditEventRecord) AuditEvent {
	description := event.Description
	if description == "" {
		endpoint := event.Endpoint
		if endpoint == "" {
			endpoint = event.Target
		}
		description = auditDescription(event.Method, endpoint)
	}
	return AuditEvent{ID: event.ID, Actor: event.ActorID, ActorName: event.ActorName, ActorEmail: event.ActorEmail, Action: event.Method, Description: description, Target: event.Target, Request: event.Endpoint, Result: event.Result, Input: event.RequestInput, Output: event.ResponseOutput, Traceback: event.Traceback, CorrelationID: event.CorrelationID, CreatedAt: event.CreatedAt.UTC().Format(time.RFC3339Nano), Before: toAuditMap(event.BeforeValue), After: toAuditMap(event.AfterValue)}
}

func toAuditMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	if result, ok := value.(map[string]any); ok {
		return result
	}
	return map[string]any{"value": value}
}

func (s *AuditQueryService) Add(event AuditEvent) {
	if event.ID == "" {
		event.ID = time.Now().UTC().Format("20060102150405.000000000")
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	endpoint := event.Request
	if endpoint == "" {
		if input, ok := event.Input.(map[string]any); ok {
			if value, ok := input["endpoint"].(string); ok {
				endpoint = value
			}
		}
	}
	if endpoint == "" {
		endpoint = event.Target
	}
	if event.Description == "" {
		event.Description = auditDescription(event.Action, endpoint)
	}
	event.Before = redactAuditMap(event.Before)
	event.After = redactAuditMap(event.After)
	event.Input = redactAuditValue(event.Input)
	event.Output = redactAuditValue(event.Output)
	s.mu.RLock()
	repository := s.repository
	s.mu.RUnlock()
	if repository != nil {
		createdAt, _ := time.Parse(time.RFC3339Nano, event.CreatedAt)
		_ = repository.Append(context.Background(), store.AuditEventRecord{ID: event.ID, ActorID: event.Actor, ActorName: event.ActorName, ActorEmail: event.ActorEmail, Method: event.Action, Description: event.Description, Endpoint: endpoint, Target: event.Target, Result: event.Result, RequestInput: event.Input, ResponseOutput: event.Output, BeforeValue: event.Before, AfterValue: event.After, Traceback: event.Traceback, CorrelationID: event.CorrelationID, CreatedAt: createdAt})
		return
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
}

func (s *AuditQueryService) query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	filters := map[string]string{
		"actor": r.URL.Query().Get("actor"), "action": r.URL.Query().Get("action"), "target": r.URL.Query().Get("target"),
		"result": r.URL.Query().Get("result"), "correlationId": r.URL.Query().Get("correlation_id"),
	}
	from, fromErr := parseAuditTime(r.URL.Query().Get("from"))
	to, toErr := parseAuditTime(r.URL.Query().Get("to"))
	if fromErr != nil || toErr != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid audit time range"})
		return
	}
	excludeTarget := strings.TrimSpace(r.URL.Query().Get("exclude_target"))
	excludeRunLogs := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("exclude_run_logs")), "true")
	s.mu.RLock()
	repository := s.repository
	s.mu.RUnlock()
	if repository != nil {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items, total, err := repository.Query(r.Context(), store.AuditFilter{Actor: filters["actor"], Action: filters["action"], Target: filters["target"], Result: filters["result"], CorrelationID: filters["correlationId"], ExcludeTarget: excludeTarget, ExcludeRunLogs: excludeRunLogs, From: from, To: to, Page: page, Limit: limit})
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "audit storage unavailable", err)
			return
		}
		result := make([]AuditEvent, 0, len(items))
		for _, item := range items {
			result = append(result, auditEventFromStore(item))
		}
		if page < 1 {
			page = 1
		}
		if limit < 1 || limit > 100 {
			limit = 50
		}
		pages := (total + limit - 1) / limit
		if pages == 0 {
			pages = 1
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": result, "page": page, "limit": limit, "total": total, "pages": pages})
		return
	}
	s.mu.RLock()
	items := make([]AuditEvent, 0, len(s.events))
	for _, event := range s.events {
		created, err := time.Parse(time.RFC3339Nano, event.CreatedAt)
		if err != nil || (excludeTarget != "" && strings.EqualFold(event.Target, excludeTarget)) || (excludeRunLogs && isRunLogAudit(event.Target, event.Request)) || !auditMatches(event, filters, created, from, to) {
			continue
		}
		items = append(items, event)
	}
	s.mu.RUnlock()
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	writePage(w, r, items)
}

func isRunLogAudit(paths ...string) bool {
	for _, path := range paths {
		path = strings.TrimSuffix(path, "/")
		if strings.HasPrefix(path, "/api/v1/runs/") && strings.Contains(path, "/logs") {
			return true
		}
	}
	return false
}

func auditMatches(event AuditEvent, filters map[string]string, created, from, to time.Time) bool {
	for key, value := range filters {
		if value == "" {
			continue
		}
		var actual string
		switch key {
		case "actor":
			actual = event.Actor
		case "action":
			actual = event.Action
		case "target":
			actual = event.Target
		case "result":
			actual = event.Result
		case "correlationId":
			actual = event.CorrelationID
		}
		if !strings.Contains(strings.ToLower(actual), strings.ToLower(value)) {
			return false
		}
	}
	return (from.IsZero() || !created.Before(from)) && (to.IsZero() || !created.After(to))
}

func parseAuditTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

func redactAuditMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		lower := strings.ToLower(key)
		if (strings.Contains(lower, "password") && lower != "passwordloginenabled") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") {
			result[key] = "[REDACTED]"
			continue
		}
		result[key] = redactAuditValue(value)
	}
	return result
}

func redactAuditValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return redactAuditMap(value)
	case []any:
		out := make([]any, len(value))
		for index, item := range value {
			out[index] = redactAuditValue(item)
		}
		return out
	default:
		return value
	}
}
