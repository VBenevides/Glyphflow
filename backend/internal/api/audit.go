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

var auditDescriptionByRequest = map[string]string{
	http.MethodGet + " /api/v1/admin/auth/providers": "List SSO providers",
	http.MethodGet + " /api/v1/admin/roles":          "List roles",
	http.MethodGet + " /api/v1/roles":                "List roles",
	http.MethodGet + " /api/v1/sso":                  "View SSO configuration",
	http.MethodGet + " /api/v1/users":                "List users",
	http.MethodGet + " /api/v1/me":                   "View own profile",
	http.MethodGet + " /api/v1/tasks":                "List tasks",
	http.MethodGet + " /api/v1/schedules":            "List schedules",
	http.MethodGet + " /api/v1/resources":            "List resources",
}

var auditDescriptionByPath = map[string]string{
	"/api/v1/admin/auth/settings":        "Update authentication settings",
	"/api/v1/admin/auth/sessions/revoke": "Revoke user session",
	"/api/v1/admin/auth/providers":       "Update SSO provider",
	"/api/v1/admin/roles":                "Create role",
	"/api/v1/roles":                      "Manage roles",
	"/api/v1/sso":                        "Update SSO configuration",
	"/api/v1/logs":                       "List logs",
	"/api/v1/events":                     "List events",
	"/api/v1/users":                      "Create user",
	"/api/v1/audit":                      "View audit events",
	"/api/v1/me":                         "Update own profile",
	"/api/v1/me/password":                "Change own password",
	"/api/v1/tasks":                      "Create task",
	"/api/v1/schedules":                  "Create schedule",
	"/api/v1/schedules/preview":          "Preview schedule occurrences",
	"/api/v1/resources":                  "Create resource",
	"/api/v1/runners":                    "List runners",
	"/api/v1/runs":                       "List runs",
	"/api/v1/runs/execute":               "Start task run",
	"/api/v1/runs/retry":                 "Retry run",
	"/api/v1/runs/cancel":                "Cancel run",
}

var auditRunnerActions = map[string]string{
	"enable":  "Enable runner",
	"disable": "Disable runner",
	"drain":   "Drain runner",
	"reset":   "Reset runner",
	"revoke":  "Revoke runner",
}

func auditDescription(method, path string) string {
	if description, ok := auditDescriptionByRequest[method+" "+path]; ok {
		return description
	}
	if description, ok := auditDescriptionByPath[path]; ok {
		return description
	}
	if description, ok := auditUserDescription(path); ok {
		return description
	}
	if description, ok := auditRoleDescription(method, path); ok {
		return description
	}
	if description, ok := auditAccountDescription(path); ok {
		return description
	}
	if description, ok := auditScheduleDescription(method, path); ok {
		return description
	}
	if description, ok := auditTaskDescription(method, path); ok {
		return description
	}
	if description, ok := auditResourceDescription(method, path); ok {
		return description
	}
	if description, ok := auditRunnerDescription(method, path); ok {
		return description
	}
	if description, ok := auditRunDescription(method, path); ok {
		return description
	}
	return strings.TrimSpace(method + " " + path)
}

func auditUserDescription(path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/admin/auth/users/") {
		return "Disable user", true
	}
	return "", false
}

func auditRoleDescription(method, path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/admin/roles/") {
		if method == http.MethodDelete {
			return "Delete role", true
		}
		return "Update role", true
	}
	return "", false
}

func auditAccountDescription(path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/users/") {
		return "View user details", true
	}
	if strings.HasPrefix(path, "/api/v1/me/") {
		return "Manage own account", true
	}
	return "", false
}

func auditScheduleDescription(method, path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/schedules/") {
		if method == http.MethodDelete {
			return "Delete schedule", true
		}
		if method == http.MethodGet {
			return "View schedule", true
		}
		return "Update schedule", true
	}
	return "", false
}

func auditTaskDescription(method, path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/tasks/") {
		if strings.HasSuffix(path, "/versions") {
			return "Publish task version", true
		}
		if strings.HasSuffix(path, "/cancel") {
			return "Cancel task run", true
		}
		if strings.HasSuffix(path, "/retry") {
			return "Retry task run", true
		}
		if method == http.MethodDelete {
			return "Delete task", true
		}
		if method == http.MethodGet {
			return "View task", true
		}
		return "Update task", true
	}
	return "", false
}

func auditResourceDescription(method, path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/resources/") {
		if strings.HasSuffix(path, "/lease") {
			if method == http.MethodPost {
				return "Acquire resource lease", true
			}
			return "Release resource lease", true
		}
		if method == http.MethodGet {
			return "View resource", true
		}
		if method == http.MethodDelete {
			return "Delete resource", true
		}
		return "Update resource", true
	}
	return "", false
}

func auditRunnerDescription(method, path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/runners/") {
		if strings.HasSuffix(path, "/enrollments") {
			return "Create runner enrollment", true
		}
		if method == http.MethodDelete {
			return "Delete runner", true
		}
		for action, description := range auditRunnerActions {
			if strings.HasSuffix(path, "/"+action) {
				return description, true
			}
		}
		if method == http.MethodGet {
			return "View runner", true
		}
		return "Update runner", true
	}
	return "", false
}

func auditRunDescription(method, path string) (string, bool) {
	if strings.HasPrefix(path, "/api/v1/runs/") {
		if strings.HasSuffix(path, "/logs/download") {
			return "Download run logs", true
		}
		if strings.HasSuffix(path, "/logs") {
			return "Stream run logs", true
		}
		if strings.HasSuffix(path, "/events") {
			return "List run events", true
		}
		if strings.HasSuffix(path, "/cancel") {
			return "Cancel run", true
		}
		if strings.HasSuffix(path, "/retry") {
			return "Retry run", true
		}
		if strings.HasSuffix(path, "/reconcile") {
			return "Reconcile run", true
		}
		if method == http.MethodGet {
			return "View run", true
		}
		return "Manage run", true
	}
	return "", false
}

type AuditQueryService struct {
	mu                   sync.RWMutex
	events               []AuditEvent
	repository           store.AuditRepository
	appendFailureHandler func(AuditEvent, error)
	liveLogAudits        map[string]time.Time
}

const auditAllLimit = 1000
const liveLogAuditWindow = 5 * time.Minute

type auditQueryOptions struct {
	filters        map[string]string
	from           time.Time
	to             time.Time
	excludeTarget  string
	excludeResult  string
	excludeMethod  string
	excludeRunLogs bool
	all            bool
	page           int
	limit          int
}

type auditPagination struct {
	page  int
	limit int
	pages int
}

func auditCounts(events []AuditEvent) store.AuditCounts {
	counts := store.AuditCounts{Total: len(events)}
	for _, event := range events {
		if strings.EqualFold(event.Result, "failure") {
			counts.Failures++
		}
		if strings.EqualFold(event.Action, http.MethodPost) || strings.EqualFold(event.Action, http.MethodPut) || strings.EqualFold(event.Action, http.MethodPatch) || strings.EqualFold(event.Action, http.MethodDelete) {
			counts.Writes++
		}
	}
	return counts
}

func NewAuditQueryService() *AuditQueryService {
	return &AuditQueryService{liveLogAudits: map[string]time.Time{}}
}

func (s *AuditQueryService) SetRepository(repository store.AuditRepository) {
	if repository != nil {
		s.mu.Lock()
		s.repository = repository
		s.mu.Unlock()
	}
}

func (s *AuditQueryService) hasDurableRepository() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.repository != nil
}

// SetAppendFailureHandler registers the operator signal for durable audit
// write failures. The callback must not block request handling.
func (s *AuditQueryService) SetAppendFailureHandler(handler func(AuditEvent, error)) {
	s.mu.Lock()
	s.appendFailureHandler = handler
	s.mu.Unlock()
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

func (s *AuditQueryService) Add(event AuditEvent) error {
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
	appendFailureHandler := s.appendFailureHandler
	s.mu.RUnlock()
	if repository != nil {
		createdAt, _ := time.Parse(time.RFC3339Nano, event.CreatedAt)
		err := repository.Append(context.Background(), store.AuditEventRecord{ID: event.ID, ActorID: event.Actor, ActorName: event.ActorName, ActorEmail: event.ActorEmail, Method: event.Action, Description: event.Description, Endpoint: endpoint, Target: event.Target, Result: event.Result, RequestInput: event.Input, ResponseOutput: event.Output, BeforeValue: event.Before, AfterValue: event.After, Traceback: event.Traceback, CorrelationID: event.CorrelationID, CreatedAt: createdAt})
		if err != nil && appendFailureHandler != nil {
			appendFailureHandler(event, err)
		}
		return err
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return nil
}

func (s *AuditQueryService) AddLiveLog(key string, event AuditEvent) error {
	s.mu.Lock()
	now := time.Now().UTC()
	if s.liveLogAudits == nil {
		s.liveLogAudits = map[string]time.Time{}
	}
	for existing, at := range s.liveLogAudits {
		if now.Sub(at) >= liveLogAuditWindow {
			delete(s.liveLogAudits, existing)
		}
	}
	if at, ok := s.liveLogAudits[key]; ok && now.Sub(at) < liveLogAuditWindow {
		s.mu.Unlock()
		return nil
	}
	s.liveLogAudits[key] = now
	s.mu.Unlock()
	if err := s.Add(event); err != nil {
		s.mu.Lock()
		delete(s.liveLogAudits, key)
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *AuditQueryService) query(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	options, ok := newAuditQueryOptions(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid audit time range"})
		return
	}
	repository := s.auditRepository()
	if repository != nil {
		s.queryAuditRepository(w, r, repository, options)
		return
	}
	s.queryInMemoryAudit(w, options)
}

func newAuditQueryOptions(r *http.Request) (auditQueryOptions, bool) {
	query := r.URL.Query()
	from, fromErr := parseAuditTime(query.Get("from"))
	to, toErr := parseAuditTime(query.Get("to"))
	if fromErr != nil || toErr != nil {
		return auditQueryOptions{}, false
	}
	filters := map[string]string{
		"actor": query.Get("actor"), "action": query.Get("action"), "target": query.Get("target"),
		"result": query.Get("result"), "correlationId": query.Get("correlation_id"),
	}
	excludeResult := strings.TrimSpace(query.Get("exclude_result"))
	if excludeResult == "" && filters["result"] == "" {
		excludeResult = "accepted"
	}
	excludeMethod := strings.TrimSpace(query.Get("exclude_method"))
	if excludeMethod == "" && !strings.EqualFold(strings.TrimSpace(query.Get("include_get")), "true") {
		excludeMethod = http.MethodGet
	}
	page, _ := strconv.Atoi(query.Get("page"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	return auditQueryOptions{
		filters: filters, from: from, to: to,
		excludeTarget: strings.TrimSpace(query.Get("exclude_target")), excludeResult: excludeResult, excludeMethod: excludeMethod,
		excludeRunLogs: strings.EqualFold(strings.TrimSpace(query.Get("exclude_run_logs")), "true"),
		all:            strings.EqualFold(strings.TrimSpace(query.Get("all")), "true"), page: page, limit: limit,
	}, true
}

func (o auditQueryOptions) storeFilter() store.AuditFilter {
	return store.AuditFilter{Actor: o.filters["actor"], Action: o.filters["action"], Target: o.filters["target"], Result: o.filters["result"], CorrelationID: o.filters["correlationId"], ExcludeMethod: o.excludeMethod, ExcludeTarget: o.excludeTarget, ExcludeResult: o.excludeResult, ExcludeRunLogs: o.excludeRunLogs, All: o.all, From: o.from, To: o.to, Page: o.page, Limit: o.limit}
}

func (s *AuditQueryService) auditRepository() store.AuditRepository {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.repository
}

func (s *AuditQueryService) queryAuditRepository(w http.ResponseWriter, r *http.Request, repository store.AuditRepository, options auditQueryOptions) {
	items, counts, err := repository.Query(r.Context(), options.storeFilter())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit storage unavailable", err)
		return
	}
	result := make([]AuditEvent, 0, len(items))
	for _, item := range items {
		result = append(result, auditEventFromStore(item))
	}
	pagination := normalizeAuditPagination(options.all, options.page, options.limit, len(result), counts.Total)
	if options.all && len(result) > pagination.limit {
		result = result[:pagination.limit]
	}
	writeAuditPage(w, result, pagination, counts)
}

func (s *AuditQueryService) queryInMemoryAudit(w http.ResponseWriter, options auditQueryOptions) {
	s.mu.RLock()
	items := make([]AuditEvent, 0, len(s.events))
	for _, event := range s.events {
		if auditEventExcluded(event, options) {
			continue
		}
		items = append(items, event)
	}
	s.mu.RUnlock()
	sort.SliceStable(items, func(i, j int) bool { return items[i].CreatedAt > items[j].CreatedAt })
	counts := auditCounts(items)
	pagination := normalizeAuditPagination(options.all, options.page, options.limit, len(items), counts.Total)
	if options.all && len(items) > pagination.limit {
		items = items[:pagination.limit]
	}
	start := (pagination.page - 1) * pagination.limit
	if start > len(items) {
		start = len(items)
	}
	end := start + pagination.limit
	if end > len(items) {
		end = len(items)
	}
	writeAuditPage(w, items[start:end], pagination, counts)
}

func auditEventExcluded(event AuditEvent, options auditQueryOptions) bool {
	created, err := time.Parse(time.RFC3339Nano, event.CreatedAt)
	if err != nil {
		return true
	}
	if options.excludeMethod != "" && strings.EqualFold(event.Action, options.excludeMethod) {
		return true
	}
	if options.excludeTarget != "" && strings.EqualFold(event.Target, options.excludeTarget) {
		return true
	}
	if options.excludeResult != "" && strings.EqualFold(event.Result, options.excludeResult) {
		return true
	}
	if options.excludeRunLogs && isRunLogAudit(event.Target, event.Request) {
		return true
	}
	return !auditMatches(event, options.filters, created, options.from, options.to)
}

func normalizeAuditPagination(all bool, page, limit, itemCount, total int) auditPagination {
	if page < 1 {
		page = 1
	}
	if all {
		page = 1
		limit = itemCount
		if limit > auditAllLimit {
			limit = auditAllLimit
		}
		if limit == 0 {
			limit = 1
		}
	} else if limit < 1 || limit > 100 {
		limit = 50
	}
	pages := (total + limit - 1) / limit
	if pages == 0 {
		pages = 1
	}
	return auditPagination{page: page, limit: limit, pages: pages}
}

func writeAuditPage(w http.ResponseWriter, items []AuditEvent, pagination auditPagination, counts store.AuditCounts) {
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "page": pagination.page, "limit": pagination.limit, "total": counts.Total, "pages": pagination.pages, "failureCount": counts.Failures, "writeCount": counts.Writes})
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
	case string:
		return redactSensitiveText(value)
	default:
		return value
	}
}

func redactSensitiveText(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"password", "token", "secret", "private_key", "private-key", "authorization", "bearer ", "api_key", "api-key"} {
		if strings.Contains(lower, marker) {
			return "[REDACTED]"
		}
	}
	return value
}
