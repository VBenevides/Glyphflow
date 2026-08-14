package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomRoleEndpointCreatesRoleWithPermissions(t *testing.T) {
	server := Server{Roles: NewRoleAdminService(), Auth: func(*http.Request) (Claims, bool) { return Claims{Roles: map[string]bool{"roles.manage": true}}, true }}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/roles", bytes.NewBufferString(`{"key":"operator","permissions":["tasks.read"]}`))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("role create: %d", w.Code)
	}
	if err := server.Roles.Assign("u", "operator"); err != nil {
		t.Fatal(err)
	}
}
