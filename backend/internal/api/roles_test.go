package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
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
	if got := server.Roles.Effective("u"); !got["tasks.read"] {
		t.Fatal("effective permission missing")
	}
	if err := server.Roles.ReplacePermissions("operator", []string{"runs.read"}); err != nil {
		t.Fatal(err)
	}
	if err := server.Roles.ReplacePermissions("operator", []string{"not-a-permission"}); err == nil {
		t.Fatal("unknown permission accepted")
	}
	if err := server.Roles.Seed("admin", platform.PermissionCatalog); err != nil {
		t.Fatal(err)
	}
	if err := server.Roles.Delete("admin"); err == nil {
		t.Fatal("system role deleted")
	}
}
