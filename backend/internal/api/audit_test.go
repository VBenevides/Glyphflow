package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type failingAuditRepository struct{ err error }

func (r failingAuditRepository) Append(context.Context, store.AuditEventRecord) error { return r.err }
func (r failingAuditRepository) Query(context.Context, store.AuditFilter) ([]store.AuditEventRecord, store.AuditCounts, error) {
	return nil, store.AuditCounts{}, r.err
}

func TestAuditAppendFailureIsSignalled(t *testing.T) {
	want := errors.New("audit database unavailable")
	audit := NewAuditQueryService()
	audit.SetRepository(failingAuditRepository{err: want})
	var signalled AuditEvent
	var got error
	audit.SetAppendFailureHandler(func(event AuditEvent, err error) {
		signalled, got = event, err
	})
	if err := audit.Add(AuditEvent{ID: "audit-failure", Actor: "user-1", Action: http.MethodPost, Target: "/api/v1/tasks"}); !errors.Is(err, want) {
		t.Fatalf("append error = %v, want %v", err, want)
	}
	if !errors.Is(got, want) || signalled.ID != "audit-failure" {
		t.Fatalf("failure signal = %#v, %v", signalled, got)
	}
}

func TestDurableAuditFailureBlocksMutation(t *testing.T) {
	audit := NewAuditQueryService()
	audit.SetRepository(failingAuditRepository{err: errors.New("audit database unavailable")})
	mutated := false
	server := Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "user-1"}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"runners.manage": true} }, AuditQuery: audit}
	handler := server.require("runners.manage", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mutated = true
		writeJSON(w, http.StatusCreated, map[string]string{"id": "runner-1"})
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", nil))
	if response.Code != http.StatusServiceUnavailable || mutated {
		t.Fatalf("audit gate: status=%d mutated=%t body=%s", response.Code, mutated, response.Body.String())
	}
}

func TestAuditQueryFiltersRedactsAndPaginates(t *testing.T) {
	audit := NewAuditQueryService()
	audit.Add(AuditEvent{ID: "old", Actor: "system:scheduler", Action: "run.created", Target: "run-1", Result: "success", CorrelationID: "corr-1", CreatedAt: "2026-08-13T10:00:00Z", Before: map[string]any{"token": "secret"}})
	audit.Add(AuditEvent{ID: "write", Actor: "user-1", Action: http.MethodPost, Target: "/api/v1/tasks", Result: "success", CreatedAt: "2026-08-09T10:00:00Z"})
	audit.Add(AuditEvent{ID: "new", Actor: "user-1", Action: "user.updated", Target: "user-1", Result: "failure", CorrelationID: "corr-2", CreatedAt: "2026-08-14T10:00:00Z", After: map[string]any{"nested": map[string]any{"password": "secret"}}})
	audit.Add(AuditEvent{ID: "audit-read", Actor: "system:audit", Action: "GET", Target: "/api/v1/audit", Result: "success", CreatedAt: "2026-08-14T11:00:00Z"})
	audit.Add(AuditEvent{ID: "task-read", Actor: "system:tasks", Action: "GET", Target: "/api/v1/tasks", Result: "success", CreatedAt: "2026-08-14T11:30:00Z"})
	audit.Add(AuditEvent{ID: "run-log-read", Actor: "system:runner", Action: "GET", Target: "/api/v1/runs/run-1/logs", Result: "success", CreatedAt: "2026-08-14T12:00:00Z"})
	if audit.events[0].Before["token"] != "[REDACTED]" || audit.events[2].After["nested"].(map[string]any)["password"] != "[REDACTED]" {
		t.Fatal("audit secrets were not redacted")
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit?actor=user-1&page=1&limit=1", nil)
	response := httptest.NewRecorder()
	audit.query(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"new"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"id":"old"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"total":2`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"failureCount":1`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"writeCount":1`)) {
		t.Fatalf("filtered audit response: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit?from="+time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"id":"new"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"id":"old"`)) {
		t.Fatalf("time-filtered audit response: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit?exclude_target=/api/v1/audit", nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if bytes.Contains(response.Body.Bytes(), []byte(`"id":"audit-read"`)) {
		t.Fatalf("excluded audit-read event returned: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if bytes.Contains(response.Body.Bytes(), []byte(`"id":"audit-read"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"id":"task-read"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"id":"run-log-read"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"write"`)) {
		t.Fatalf("GET events were not excluded by default: %s", response.Body.String())
	}
	var filtered struct {
		Total, FailureCount, WriteCount int
	}
	if err := json.Unmarshal(response.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 3 || filtered.FailureCount != 1 || filtered.WriteCount != 1 {
		t.Fatalf("default audit counts = %+v, want total=3 failures=1 writes=1", filtered)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit?exclude_run_logs=true", nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if bytes.Contains(response.Body.Bytes(), []byte(`"id":"run-log-read"`)) {
		t.Fatalf("run-log event returned with exclusion: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if bytes.Contains(response.Body.Bytes(), []byte(`"id":"run-log-read"`)) {
		t.Fatalf("run-log GET event returned by default: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit?include_get=true", nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"id":"audit-read"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"task-read"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"run-log-read"`)) {
		t.Fatalf("GET events missing with explicit opt-in: %s", response.Body.String())
	}
}

func TestAuditHidesAcceptedPreflightByDefault(t *testing.T) {
	audit := NewAuditQueryService()
	audit.Add(AuditEvent{ID: "accepted", Action: http.MethodPost, Target: "/api/v1/tasks", Result: "accepted"})
	audit.Add(AuditEvent{ID: "success", Action: http.MethodPost, Target: "/api/v1/tasks", Result: "success"})

	response := httptest.NewRecorder()
	audit.query(response, httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil))
	if response.Code != http.StatusOK || bytes.Contains(response.Body.Bytes(), []byte(`"id":"accepted"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"success"`)) || !bytes.Contains(response.Body.Bytes(), []byte(`"total":1`)) {
		t.Fatalf("default audit events = %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	audit.query(response, httptest.NewRequest(http.MethodGet, "/api/v1/audit?result=accepted", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"accepted"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"id":"success"`)) {
		t.Fatalf("accepted audit events = %s", response.Body.String())
	}
}

type recordingAuditRepository struct {
	filter store.AuditFilter
}

func (r *recordingAuditRepository) Append(context.Context, store.AuditEventRecord) error { return nil }
func (r *recordingAuditRepository) Query(_ context.Context, filter store.AuditFilter) ([]store.AuditEventRecord, store.AuditCounts, error) {
	r.filter = filter
	return nil, store.AuditCounts{}, nil
}

func TestAuditQueryPassesDefaultAndOptInMethodFiltersToRepository(t *testing.T) {
	repository := &recordingAuditRepository{}
	audit := NewAuditQueryService()
	audit.SetRepository(repository)
	response := httptest.NewRecorder()
	audit.query(response, httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil))
	if response.Code != http.StatusOK || repository.filter.ExcludeMethod != http.MethodGet {
		t.Fatalf("default repository filter = %+v, status=%d", repository.filter, response.Code)
	}
	response = httptest.NewRecorder()
	audit.query(response, httptest.NewRequest(http.MethodGet, "/api/v1/audit?include_get=true", nil))
	if response.Code != http.StatusOK || repository.filter.ExcludeMethod != "" {
		t.Fatalf("opt-in repository filter = %+v, status=%d", repository.filter, response.Code)
	}
}

func TestAuditDetailsRedactInputAndOutput(t *testing.T) {
	audit := NewAuditQueryService()
	audit.Add(AuditEvent{Input: map[string]any{"password": "secret", "nested": []any{map[string]any{"token": "secret"}}}, Output: map[string]any{"status": "ok"}, Traceback: "trace"})
	event := audit.events[0]
	input := event.Input.(map[string]any)
	if input["password"] != "[REDACTED]" || input["nested"].([]any)[0].(map[string]any)["token"] != "[REDACTED]" || event.Output.(map[string]any)["status"] != "ok" || event.Traceback != "trace" {
		t.Fatalf("audit details were not preserved safely: %#v", event)
	}
}

func TestAuditAllExportIsBounded(t *testing.T) {
	audit := NewAuditQueryService()
	for i := 0; i <= auditAllLimit; i++ {
		audit.Add(AuditEvent{ID: fmt.Sprintf("audit-%04d", i), CreatedAt: fmt.Sprintf("2026-08-14T10:%02d:00Z", i%60)})
	}
	response := httptest.NewRecorder()
	audit.query(response, httptest.NewRequest(http.MethodGet, "/api/v1/audit?all=true", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("audit export status = %d", response.Code)
	}
	var page struct {
		Items []AuditEvent `json:"items"`
		Total int          `json:"total"`
	}
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != auditAllLimit || page.Total != auditAllLimit+1 {
		t.Fatalf("audit export = %d items, total %d", len(page.Items), page.Total)
	}
}

func TestAuditFailureStoresErrorAndTraceback(t *testing.T) {
	audit := NewAuditQueryService()
	server := Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "user-1"}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"runners.manage": true} }, AuditQuery: audit}
	handler := server.require("runners.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recordRequestError(r, errors.New("database enrollment constraint failed"))
		writeJSON(w, http.StatusConflict, map[string]string{"error": "enrollment failed"})
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", nil))
	if response.Code != http.StatusConflict || len(audit.events) != 1 {
		t.Fatalf("audit failure: status=%d events=%d", response.Code, len(audit.events))
	}
	event := audit.events[0]
	output := event.Output.(map[string]any)
	if output["error"] != "database enrollment constraint failed" || !strings.Contains(event.Traceback, "database enrollment constraint failed") || !strings.Contains(event.Traceback, "recordRequestError") {
		t.Fatalf("audit error details missing: %#v", event)
	}
}

func TestAuditCapturesEndpointMethodAndBody(t *testing.T) {
	audit := NewAuditQueryService()
	server := Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "user-1"}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"runners.manage": true} }, AuditQuery: audit}
	handler := server.require("runners.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		writeJSON(w, http.StatusOK, body)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"name":"Task","token":"secret"}`)))
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d", len(audit.events))
	}
	input, ok := audit.events[0].Input.(map[string]any)
	if !ok || input["endpoint"] != "/api/v1/tasks" || input["method"] != http.MethodPost {
		t.Fatalf("audit input metadata = %#v", audit.events[0].Input)
	}
	body, ok := input["body"].(map[string]any)
	if !ok || body["name"] != "Task" || body["token"] != "[REDACTED]" {
		t.Fatalf("audit input body = %#v", input["body"])
	}
	output, ok := audit.events[0].Output.(map[string]any)
	if !ok || output["status"] != http.StatusOK {
		t.Fatalf("audit output metadata = %#v", audit.events[0].Output)
	}
	responseBody, ok := output["body"].(map[string]any)
	if !ok || responseBody["name"] != "Task" || responseBody["token"] != "[REDACTED]" {
		t.Fatalf("audit output body = %#v", output["body"])
	}
}

func TestAuditCapturesMutationBeforeAndAfter(t *testing.T) {
	globalVariables := NewGlobalVariableService()
	globalVariables.items["global-1"] = store.GlobalVariableRecord{ID: "global-1", Name: "NAME", Value: "before"}
	audit := NewAuditQueryService()
	server := Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "user-1"}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"users.manage": true} }, AuditQuery: audit, GlobalVariables: globalVariables}
	request := httptest.NewRequest(http.MethodPut, "/api/v1/global-variables/global-1", bytes.NewBufferString(`{"name":"NAME","value":"after"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(audit.events) != 1 {
		t.Fatalf("global variable update: status=%d events=%d", response.Code, len(audit.events))
	}
	event := audit.events[0]
	if event.Before["value"] != "before" || event.After["value"] != "after" {
		t.Fatalf("mutation snapshots = before=%#v after=%#v", event.Before, event.After)
	}
}

func TestAuditFailureCapturesResponseError(t *testing.T) {
	audit := NewAuditQueryService()
	server := Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "user-1"}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"runners.manage": true} }, AuditQuery: audit}
	handler := server.require("runners.manage", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "runner enrollment conflict"})
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", nil))
	if len(audit.events) != 1 {
		t.Fatalf("audit events = %d", len(audit.events))
	}
	event := audit.events[0]
	output := event.Output.(map[string]any)
	if output["error"] != "runner enrollment conflict" || !strings.Contains(event.Traceback, "runner enrollment conflict") {
		t.Fatalf("response error missing from audit: %#v", event)
	}
}

func TestAuditEventsIncludeUserActorDisplayData(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "users.read", "audit.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("actor@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	audit := NewAuditQueryService()
	server := Server{AuthService: auth, Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: user.ID}, true }, Permissions: auth.Permissions, AuditQuery: audit}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(audit.events) != 1 {
		t.Fatalf("user list audit: %d %d", response.Code, len(audit.events))
	}
	if audit.events[0].ActorName != user.Username || audit.events[0].ActorEmail != user.Email || audit.events[0].Description != "List users" {
		t.Fatalf("actor display data: %#v", audit.events[0])
	}
}

func TestAuditDescriptionsCoverAPIRequests(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{http.MethodPost, "/api/v1/admin/auth/sessions/revoke", "Revoke user session"},
		{http.MethodGet, "/api/v1/schedules/schedule-1", "View schedule"},
		{http.MethodDelete, "/api/v1/schedules/schedule-1", "Delete schedule"},
		{http.MethodPost, "/api/v1/tasks/task-1/versions", "Publish task version"},
		{http.MethodDelete, "/api/v1/tasks/task-1", "Delete task"},
		{http.MethodPost, "/api/v1/resources/resource-1/lease", "Acquire resource lease"},
		{http.MethodPost, "/api/v1/runners/runner-1/drain", "Drain runner"},
		{http.MethodDelete, "/api/v1/runners/runner-1", "Delete runner"},
		{http.MethodGet, "/api/v1/runs/run-1/logs/download", "Download run logs"},
		{http.MethodGet, "/api/v1/unknown", "GET /api/v1/unknown"},
	}
	for _, test := range cases {
		if got := auditDescription(test.method, test.path); got != test.want {
			t.Errorf("auditDescription(%q, %q) = %q, want %q", test.method, test.path, got, test.want)
		}
	}
	audit := NewAuditQueryService()
	audit.Add(AuditEvent{Action: http.MethodPost, Input: map[string]any{"endpoint": "/api/v1/runners/enrollments"}})
	if audit.events[0].Description != "Create runner enrollment" {
		t.Fatalf("description inferred from request input: %q", audit.events[0].Description)
	}
}
