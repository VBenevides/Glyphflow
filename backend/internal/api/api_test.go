package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestTaskCreationDoesNotAcceptUnstoredRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil)
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
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("stub task creation returned %d", response.Code)
	}
}

func TestManagementRoutesFailClosedUntilBackedByStorage(t *testing.T) {
	h := (Server{Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"run.read": true, "event.read": true, "runner.read": true, "run.cancel": true}}, true
	}}).Handler()
	for _, path := range []string{"/api/v1/runs", "/api/v1/events", "/api/v1/runners", "/api/v1/tasks/run-1/cancel"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusNotImplemented {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
}

func TestFrontendResourceAndRunPathsReachClassifiedHandlers(t *testing.T) {
	permissions := map[string]bool{"task.read": true, "task.manage": true, "tasks.read": true, "tasks.manage": true, "resources.read": true, "resources.manage": true, "runners.read": true, "runners.manage": true, "runs.read": true, "runs.cancel": true, "runs.retry": true, "logs.read": true}
	h := (Server{Auth: func(*http.Request) (Claims, bool) { return Claims{Roles: permissions}, true }}).Handler()
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
		{http.MethodPost, "/api/v1/runs/run-1/cancel"},
		{http.MethodGet, "/api/v1/runs/run-1/logs"},
		{http.MethodGet, "/api/v1/runs/run-1/logs/download"},
	}
	for _, item := range requests {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, httptest.NewRequest(item.method, item.path, nil))
		if response.Code == http.StatusNotFound {
			t.Fatalf("%s %s was not registered", item.method, item.path)
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
