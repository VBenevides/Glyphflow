package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRepresentativeRoutesAreRegistered(t *testing.T) {
	handler := (Server{}).Handler()
	for _, test := range []struct {
		path   string
		status int
	}{
		{path: "/api/v1/healthz", status: http.StatusOK},
		{path: "/api/v1/readyz", status: http.StatusOK},
		{path: "/api/v1/tasks", status: http.StatusUnauthorized},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.status {
			t.Fatalf("GET %s status = %d, want %d", test.path, response.Code, test.status)
		}
	}
}

func TestRepresentativeRoutePermissionsAndStatuses(t *testing.T) {
	authenticated := false
	granted := false
	handler := (Server{
		Auth: func(*http.Request) (Claims, bool) {
			return Claims{}, authenticated
		},
		Permissions: func(Claims) map[string]bool {
			if granted {
				return map[string]bool{"tasks.read": true}
			}
			return nil
		},
	}).Handler()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	for _, test := range []struct {
		name   string
		status int
		set    func()
	}{
		{name: "unauthenticated", status: http.StatusUnauthorized, set: func() { authenticated, granted = false, false }},
		{name: "forbidden", status: http.StatusForbidden, set: func() { authenticated, granted = true, false }},
		{name: "allowed", status: http.StatusOK, set: func() { authenticated, granted = true, true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.set()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestTaskJSONMatchesFrontendContract(t *testing.T) {
	handler := (Server{
		Auth: func(*http.Request) (Claims, bool) {
			return Claims{Roles: map[string]bool{"tasks.manage": true}}, true
		},
	}).Handler()

	requestBody := []byte(`{"name":"demo","command":["echo","ok"],"runner_pool":"default","timeout_seconds":30}`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", bytes.NewReader(requestBody)))
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /api/v1/tasks status = %d, body = %s", response.Code, response.Body.String())
	}

	var task struct {
		ID             string   `json:"id"`
		Name           string   `json:"name"`
		Pool           string   `json:"pool"`
		TimeoutSeconds int      `json:"timeoutSeconds"`
		Command        []string `json:"command"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Name != "demo" || task.Pool != "default" || task.TimeoutSeconds != 30 || len(task.Command) != 2 {
		t.Fatalf("task response = %+v", task)
	}
}
