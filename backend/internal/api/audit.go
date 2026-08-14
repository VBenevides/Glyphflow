package api

import (
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
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
	case path == "/api/v1/runs/execute":
		return "Execute task"
	case strings.HasPrefix(path, "/api/v1/runs/"):
		return "Manage run"
	default:
		return method + " " + path
	}
}

type AuditQueryService struct {
	mu     sync.RWMutex
	events []AuditEvent
}

func NewAuditQueryService() *AuditQueryService { return &AuditQueryService{} }

func (s *AuditQueryService) Add(event AuditEvent) {
	if event.ID == "" {
		event.ID = time.Now().UTC().Format("20060102150405.000000000")
	}
	if event.CreatedAt == "" {
		event.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	event.Before = redactAuditMap(event.Before)
	event.After = redactAuditMap(event.After)
	event.Input = redactAuditValue(event.Input)
	event.Output = redactAuditValue(event.Output)
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
	s.mu.RLock()
	items := make([]AuditEvent, 0, len(s.events))
	excludeTarget := strings.TrimSpace(r.URL.Query().Get("exclude_target"))
	for _, event := range s.events {
		created, err := time.Parse(time.RFC3339Nano, event.CreatedAt)
		if err != nil || (excludeTarget != "" && strings.EqualFold(event.Target, excludeTarget)) || !auditMatches(event, filters, created, from, to) {
			continue
		}
		items = append(items, event)
	}
	s.mu.RUnlock()
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	writePage(w, r, items)
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
