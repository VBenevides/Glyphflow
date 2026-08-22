package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthAndPagination(t *testing.T) {
	s := Server{Auth: func(r *http.Request) (Claims, bool) { return Claims{Roles: map[string]bool{"task.read": true}}, true }}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?page=2", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected response: %d", w.Code)
	}
}

func TestCORSOnlyAllowsExactConfiguredOrigins(t *testing.T) {
	h := (Server{CORSOrigins: []string{"http://localhost:5173", "*"}}).Handler()
	for _, test := range []struct {
		name   string
		origin string
		allow  string
	}{
		{name: "trusted origin", origin: "http://localhost:5173", allow: "http://localhost:5173"},
		{name: "untrusted origin", origin: "http://other.example"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			if got := response.Header().Get("Access-Control-Allow-Origin"); got != test.allow {
				t.Fatalf("allow origin = %q, want %q", got, test.allow)
			}
			wantCredentials := ""
			if test.allow != "" {
				wantCredentials = "true"
			}
			if got := response.Header().Get("Access-Control-Allow-Credentials"); got != wantCredentials {
				t.Fatalf("allow credentials = %q, want %q", got, wantCredentials)
			}
		})
	}
}

func TestCORSDoesNotBypassCSRFForUntrustedOrigin(t *testing.T) {
	h := (Server{
		CORSOrigins: []string{"*"},
		CSRFOrigins: []string{"http://localhost:5173"},
	}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil)
	request.Header.Set("Origin", "http://other.example")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("untrusted state-changing request returned %d", response.Code)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "" || response.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("untrusted origin received CORS headers: %#v", response.Header())
	}
}

func TestRequestBodiesAreBoundBeforePublicAndAuthenticatedHandlers(t *testing.T) {
	large := strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1))
	response := httptest.NewRecorder()
	(Server{}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", large))
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("public oversized request returned %d", response.Code)
	}
	sessions, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := sessions.Issue("user-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(strings.Repeat("x", maxRequestBodyBytes+1)))
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	(Server{Auth: sessions.Authenticator(), Permissions: func(Claims) map[string]bool { return map[string]bool{"task.create": true} }}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("authenticated oversized request returned %d", response.Code)
	}
}

func TestCorrelationIDIsGeneratedAndAudited(t *testing.T) {
	audit := NewAuditQueryService()
	h := (Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: "user-1"}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"task.read": true} }, AuditQuery: audit}).Handler()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	correlationID := response.Header().Get("X-Correlation-ID")
	if correlationID == "" || len(audit.events) != 1 || audit.events[0].CorrelationID != correlationID {
		t.Fatalf("correlation ID was not shared with audit: header=%q events=%#v", correlationID, audit.events)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	request.Header.Set("X-Correlation-ID", "client-correlation")
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Header().Get("X-Correlation-ID") != "client-correlation" {
		t.Fatalf("provided correlation ID was not preserved: %q", response.Header().Get("X-Correlation-ID"))
	}
}

func TestSessionAuthenticatorAndCreateRole(t *testing.T) {
	sessions, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := sessions.Issue("user-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	h := Server{Auth: sessions.Authenticator(), Permissions: func(Claims) map[string]bool { return map[string]bool{"tasks.read": true, "tasks.manage": true} }}.Handler()
	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer token returned %d", unauthorized.Code)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/api/v1/tasks", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code == http.StatusUnauthorized || response.Code == http.StatusForbidden {
			t.Fatalf("valid bearer token rejected for %s: %d", method, response.Code)
		}
	}
}

func TestTaskCreationStoresAndReturnsTask(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"name":"Nightly","command":["echo","hi"],"runner_pool":"default","timeout_seconds":30}`))
	sessions, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := sessions.Issue("user-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	(Server{Auth: sessions.Authenticator(), Permissions: func(Claims) map[string]bool { return map[string]bool{"tasks.manage": true} }}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("task creation returned %d", response.Code)
	}
}

func TestPluralTaskManagePermissionAuthorizesTaskMutations(t *testing.T) {
	h := (Server{Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"tasks.manage": true}}, true
	}}).Handler()
	create := httptest.NewRecorder()
	h.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"name":"Nightly","command":["echo","hi"],"runner_pool":"default"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("task creation returned %d", create.Code)
	}
	version := httptest.NewRecorder()
	h.ServeHTTP(version, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/versions", bytes.NewBufferString(`{"command":["echo","updated"],"runner_pool":"default"}`)))
	if version.Code != http.StatusCreated {
		t.Fatalf("task version mutation returned %d", version.Code)
	}
	deleted := httptest.NewRecorder()
	h.ServeHTTP(deleted, httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/task-1", nil))
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("task deletion returned %d", deleted.Code)
	}
}

func TestRunnerPoolCRUD(t *testing.T) {
	h := (Server{Auth: func(*http.Request) (Claims, bool) { return Claims{}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"runners.read": true, "runners.manage": true} }, Infrastructure: NewInfrastructureService()}).Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return response
	}
	if response := request(http.MethodGet, "/api/v1/runners/pools", ""); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"default"`)) {
		t.Fatalf("pool listing returned %d: %s", response.Code, response.Body.String())
	}
	response := request(http.MethodPost, "/api/v1/runners/pools", `{"name":"build","description":"Build workers"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("pool creation returned %d: %s", response.Code, response.Body.String())
	}
	var pool RunnerPoolRecord
	if err := json.Unmarshal(response.Body.Bytes(), &pool); err != nil || pool.ID == "" {
		t.Fatalf("created pool: %s", response.Body.String())
	}
	if response = request(http.MethodPut, "/api/v1/runners/pools/"+pool.ID, `{"name":"release","enabled":true}`); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"release"`)) {
		t.Fatalf("pool update returned %d: %s", response.Code, response.Body.String())
	}
	if response = request(http.MethodDelete, "/api/v1/runners/pools/"+pool.ID, ""); response.Code != http.StatusNoContent {
		t.Fatalf("pool deletion returned %d: %s", response.Code, response.Body.String())
	}
}

func TestInMemoryTaskAndScheduleDeletion(t *testing.T) {
	permissions := map[string]bool{"task.create": true, "task.manage": true, "task.read": true, "tasks.manage": true, "tasks.read": true}
	h := (Server{Auth: func(*http.Request) (Claims, bool) { return Claims{Roles: permissions}, true }}).Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return response
	}
	if response := request(http.MethodPost, "/api/v1/tasks", `{"name":"Nightly","command":["echo","hi"],"runner_pool":"default"}`); response.Code != http.StatusCreated {
		t.Fatalf("task creation returned %d", response.Code)
	}
	if response := request(http.MethodPost, "/api/v1/schedules", `{"name":"Hourly","task_id":"task-1","schedule_type":"cron","expression":"0 * * * *","timezone":"UTC"}`); response.Code != http.StatusCreated {
		t.Fatalf("schedule creation returned %d", response.Code)
	}
	if response := request(http.MethodDelete, "/api/v1/schedules/schedule-1", ""); response.Code != http.StatusNoContent {
		t.Fatalf("schedule deletion returned %d", response.Code)
	}
	if response := request(http.MethodPost, "/api/v1/schedules", `{"name":"Hourly again","task_id":"task-1","schedule_type":"cron","expression":"0 * * * *","timezone":"UTC"}`); response.Code != http.StatusCreated {
		t.Fatalf("second schedule creation returned %d", response.Code)
	}
	if response := request(http.MethodDelete, "/api/v1/tasks/task-1", ""); response.Code != http.StatusNoContent {
		t.Fatalf("task deletion returned %d", response.Code)
	}
	if response := request(http.MethodGet, "/api/v1/schedules/schedule-2", ""); response.Code != http.StatusNotFound {
		t.Fatalf("task deletion left schedule with status %d", response.Code)
	}
	if response := request(http.MethodDelete, "/api/v1/tasks/task-1", ""); response.Code != http.StatusNotFound {
		t.Fatalf("missing task deletion returned %d", response.Code)
	}
}

func TestScheduleEnableDisableDoesNotCreateVersion(t *testing.T) {
	permissions := map[string]bool{"tasks.manage": true, "tasks.read": true}
	h := (Server{Auth: func(*http.Request) (Claims, bool) { return Claims{Roles: permissions}, true }}).Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return response
	}
	if response := request(http.MethodPost, "/api/v1/tasks", `{"name":"Nightly","command":["echo","ok"],"runner_pool":"default"}`); response.Code != http.StatusCreated {
		t.Fatalf("task creation returned %d", response.Code)
	}
	if response := request(http.MethodPost, "/api/v1/schedules", `{"name":"Hourly","task_id":"task-1","expression":"0 * * * *","timezone":"UTC"}`); response.Code != http.StatusCreated {
		t.Fatalf("schedule creation returned %d", response.Code)
	}
	if response := request(http.MethodPost, "/api/v1/schedules/schedule-1/disable", ""); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"state":"DISABLED"`)) {
		t.Fatalf("schedule disable returned %d: %s", response.Code, response.Body.String())
	}
	if response := request(http.MethodGet, "/api/v1/schedules/schedule-1", ""); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"expression":"0 * * * *"`)) {
		t.Fatalf("schedule definition changed: %d %s", response.Code, response.Body.String())
	}
}

func TestManagementRoutesFailClosedUntilBackedByStorage(t *testing.T) {
	h := (Server{Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"run.read": true, "event.read": true, "runner.read": true, "runners.read": true, "run.cancel": true}}, true
	}}).Handler()
	for _, item := range []struct{ method, path string }{{http.MethodGet, "/api/v1/runs"}, {http.MethodGet, "/api/v1/events"}, {http.MethodGet, "/api/v1/runners"}, {http.MethodPost, "/api/v1/tasks/run-1/cancel"}} {
		r := httptest.NewRequest(item.method, item.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if (item.path == "/api/v1/runs" || item.path == "/api/v1/runners") && w.Code != http.StatusOK {
			t.Fatalf("run listing returned %d", w.Code)
		}
		if item.path == "/api/v1/tasks/run-1/cancel" && w.Code != http.StatusNotFound {
			t.Fatalf("missing task action returned %d", w.Code)
		}
		if item.path != "/api/v1/tasks/run-1/cancel" && item.path != "/api/v1/runs" && item.path != "/api/v1/runners" && w.Code != http.StatusNotImplemented {
			t.Fatalf("%s returned %d", item.path, w.Code)
		}
	}
}

func TestFrontendResourceAndRunPathsReachClassifiedHandlers(t *testing.T) {
	permissions := map[string]bool{"task.read": true, "task.manage": true, "tasks.read": true, "tasks.manage": true, "resources.read": true, "resources.manage": true, "runners.read": true, "runners.manage": true, "runs.read": true, "runs.execute": true, "runs.cancel": true, "runs.retry": true, "logs.read": true}
	infrastructure := NewInfrastructureService()
	infrastructure.resources["resource-1"] = ResourceRecord{ID: "resource-1", Name: "resource-1", Enabled: true}
	h := (Server{Auth: func(*http.Request) (Claims, bool) { return Claims{Roles: permissions}, true }, Infrastructure: infrastructure}).Handler()
	createTask := httptest.NewRecorder()
	h.ServeHTTP(createTask, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewBufferString(`{"name":"Nightly","command":["echo","hi"],"runner_pool":"default"}`)))
	createSchedule := httptest.NewRecorder()
	h.ServeHTTP(createSchedule, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewBufferString(`{"name":"Hourly","task_id":"task-1","schedule_type":"cron","expression":"0 * * * *","timezone":"UTC"}`)))
	if createTask.Code != http.StatusCreated || createSchedule.Code != http.StatusCreated {
		t.Fatalf("seed operations: task=%d schedule=%d", createTask.Code, createSchedule.Code)
	}
	createRun := httptest.NewRecorder()
	h.ServeHTTP(createRun, httptest.NewRequest(http.MethodPost, "/api/v1/runs/execute", bytes.NewBufferString(`{"task_id":"task-1"}`)))
	if createRun.Code != http.StatusCreated {
		t.Fatalf("seed run: %d", createRun.Code)
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createRun.Body).Decode(&run); err != nil || run.ID == "" {
		t.Fatalf("seed run response: %s", createRun.Body.String())
	}
	requests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/tasks/task-1"},
		{http.MethodPost, "/api/v1/tasks/task-1/versions"},
		{http.MethodPost, "/api/v1/schedules/preview"},
		{http.MethodGet, "/api/v1/schedules/schedule-1"},
		{http.MethodGet, "/api/v1/resources/resource-1"},
		{http.MethodPost, "/api/v1/runners/enrollments"},
		{http.MethodPost, "/api/v1/runs/" + run.ID + "/cancel"},
		{http.MethodGet, "/api/v1/runs/" + run.ID + "/logs"},
		{http.MethodGet, "/api/v1/runs/" + run.ID + "/logs/download"},
	}
	for _, item := range requests {
		response := httptest.NewRecorder()
		body := bytes.NewBufferString(`{"name":"Nightly","command":["echo","hi"],"runner_pool":"default"}`)
		if item.path == "/api/v1/schedules/preview" {
			body = bytes.NewBufferString(`{"schedule_type":"cron","expression":"0 * * * *","timezone":"UTC"}`)
		}
		h.ServeHTTP(response, httptest.NewRequest(item.method, item.path, body))
		if response.Code == http.StatusNotFound || response.Code == http.StatusMethodNotAllowed {
			t.Fatalf("%s %s returned %d", item.method, item.path, response.Code)
		}
	}
}

func TestReadinessReportsDependencyFailure(t *testing.T) {
	h := (Server{Ready: func(_ context.Context) error { return errors.New("database down") }}).Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/readyz", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness returned %d", w.Code)
	}
}
