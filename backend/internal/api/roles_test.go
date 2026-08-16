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
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/roles", bytes.NewBufferString(`{"name":"operator","permissions":["tasks.read"]}`))
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

func TestRoleAndUserListsAllowReadOrManagePermission(t *testing.T) {
	roles := NewRoleAdminService()
	if err := roles.Seed("viewer", []string{"users.read"}); err != nil {
		t.Fatal(err)
	}
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	if _, err := auth.Register("user@example.com", "correct horse"); err != nil {
		t.Fatal(err)
	}

	server := Server{
		Roles:     roles,
		AuthAdmin: &AuthAdminService{Auth: auth},
		Auth: func(*http.Request) (Claims, bool) {
			return Claims{Roles: map[string]bool{"roles.read": true, "users.manage": true}}, true
		},
	}
	for _, path := range []string{"/api/v1/admin/roles", "/api/v1/users"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("read %s returned %d", path, recorder.Code)
		}
	}
}

func TestRoleListSerializesEmptyPermissionsAsArray(t *testing.T) {
	roles := NewRoleAdminService()
	if err := roles.Seed("user", nil); err != nil {
		t.Fatal(err)
	}
	server := Server{Roles: roles, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"roles.read": true}}, true
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/roles", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"permissions":[]`)) {
		t.Fatalf("role permissions contract: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestRoleListCountsDistinctAffectedUsers(t *testing.T) {
	roles := NewRoleAdminService()
	if err := roles.Seed("admin", nil); err != nil {
		t.Fatal(err)
	}
	if err := roles.Seed("user", nil); err != nil {
		t.Fatal(err)
	}
	if err := roles.Assign("default-user", "admin"); err != nil {
		t.Fatal(err)
	}
	if err := roles.Assign("default-user", "user"); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, role := range roles.List() {
		counts[role.Name] = role.AssignedUsers
	}
	if counts["admin"] != 1 || counts["user"] != 1 {
		t.Fatalf("affected users = %#v", counts)
	}
}

func TestDefaultUserRoleCannotManageInfrastructure(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", platform.UserPermissionCatalog...); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("operator", platform.OperatorPermissionCatalog...); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("regular@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	permissions := auth.Permissions(Claims{UserID: user.ID})
	for _, permission := range []string{"tasks.manage", "resources.manage", "runners.manage"} {
		if permissions[permission] {
			t.Fatalf("default user role grants %s", permission)
		}
	}
	for _, permission := range []string{"tasks.read", "runs.read", "runs.execute", "resources.read", "runners.read"} {
		if !permissions[permission] {
			t.Fatalf("default user role is missing %s", permission)
		}
	}
	server := Server{Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: user.ID}, true }, Permissions: auth.Permissions}
	for _, path := range []string{"/api/v1/tasks", "/api/v1/resources", "/api/v1/runners"} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("default user can mutate %s: %d", path, response.Code)
		}
	}
}
