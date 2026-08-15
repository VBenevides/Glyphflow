package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type testExitCodes struct{}

func (testExitCodes) List(context.Context) ([]store.ExitCodeRecord, error) {
	return []store.ExitCodeRecord{{Code: 0, Meaning: "Success"}}, nil
}

func (testExitCodes) Create(context.Context, int, string) (store.ExitCodeRecord, error) {
	return store.ExitCodeRecord{}, nil
}

func (testExitCodes) Update(context.Context, int, int, string) (store.ExitCodeRecord, error) {
	return store.ExitCodeRecord{}, nil
}

func (testExitCodes) Delete(context.Context, int) error { return nil }

func TestExecutionStatusRouteListsExitCodes(t *testing.T) {
	handler := (Server{
		Auth: func(*http.Request) (Claims, bool) {
			return Claims{Roles: map[string]bool{"auth.settings.manage": true}}, true
		},
		ExitCodes: testExitCodes{},
	}).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/execution-status", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var items []ExitCode
	if err := json.Unmarshal(response.Body.Bytes(), &items); err != nil || len(items) != 1 || items[0].Code != 0 {
		t.Fatalf("exit codes = %#v, err = %v", items, err)
	}
}
