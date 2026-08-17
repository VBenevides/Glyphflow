package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGlobalVariableCRUD(t *testing.T) {
	service := NewGlobalVariableService()
	handler := (Server{
		GlobalVariables: service,
		Auth:            func(*http.Request) (Claims, bool) { return Claims{UserID: "user-1"}, true },
		Permissions:     func(Claims) map[string]bool { return map[string]bool{"tasks.read": true, "users.manage": true} },
	}).Handler()
	create := httptest.NewRecorder()
	handler.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/global-variables", bytes.NewBufferString(`{"name":"CACHE_PATH","value":"/tmp/cache"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", create.Code, create.Body.String())
	}
	var item struct{ ID, Name, Value string }
	if err := json.Unmarshal(create.Body.Bytes(), &item); err != nil || item.ID == "" || item.Name != "CACHE_PATH" {
		t.Fatalf("created item = %#v, err = %v", item, err)
	}
	update := httptest.NewRecorder()
	handler.ServeHTTP(update, httptest.NewRequest(http.MethodPut, "/api/v1/global-variables/"+item.ID, bytes.NewBufferString(`{"name":"CACHE_PATH","value":"/var/cache"}`)))
	if update.Code != http.StatusOK || !bytes.Contains(update.Body.Bytes(), []byte(`/var/cache`)) {
		t.Fatalf("update status = %d, body = %s", update.Code, update.Body.String())
	}
	remove := httptest.NewRecorder()
	handler.ServeHTTP(remove, httptest.NewRequest(http.MethodDelete, "/api/v1/global-variables/"+item.ID, nil))
	if remove.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", remove.Code, remove.Body.String())
	}
}

func TestGlobalVariableRejectsInvalidName(t *testing.T) {
	handler := (Server{
		Auth:        func(*http.Request) (Claims, bool) { return Claims{}, true },
		Permissions: func(Claims) map[string]bool { return map[string]bool{"users.manage": true} },
	}).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/global-variables", bytes.NewBufferString(`{"name":"BAD-NAME","value":"x"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid name status = %d", response.Code)
	}
}

func TestGlobalVariableManagementRequiresAdminPermission(t *testing.T) {
	handler := (Server{
		Auth:        func(*http.Request) (Claims, bool) { return Claims{}, true },
		Permissions: func(Claims) map[string]bool { return map[string]bool{"tasks.read": true} },
	}).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/global-variables", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("global variable page status = %d", response.Code)
	}
	options := httptest.NewRecorder()
	handler.ServeHTTP(options, httptest.NewRequest(http.MethodGet, "/api/v1/global-variables/options", nil))
	if options.Code != http.StatusOK {
		t.Fatalf("global variable options status = %d", options.Code)
	}
}
