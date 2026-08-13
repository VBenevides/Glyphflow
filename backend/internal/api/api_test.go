package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestBearerAuthenticatorAndCreateRole(t *testing.T) {
	h := Server{Auth: BearerAuthenticator("secret")}.Handler()
	unauthorized := httptest.NewRecorder()
	h.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer token returned %d", unauthorized.Code)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		request := httptest.NewRequest(method, "/api/v1/tasks", nil)
		request.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code == http.StatusUnauthorized || response.Code == http.StatusForbidden {
			t.Fatalf("valid bearer token rejected for %s: %d", method, response.Code)
		}
	}
}

func TestTaskCreationDoesNotAcceptUnstoredRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	(Server{Auth: BearerAuthenticator("secret")}).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("stub task creation returned %d", response.Code)
	}
}
