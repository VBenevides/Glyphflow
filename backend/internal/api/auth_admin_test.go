package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationAdministrationRequiresPermission(t *testing.T) {
	password := NewPasswordAuthService(true, false, nil)
	admin := &AuthAdminService{Password: password}
	server := Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"auth.settings.manage": true}}, true
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/settings", bytes.NewBufferString(`{"enabled":false}`))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || password.enabled {
		t.Fatalf("settings update failed: %d", w.Code)
	}
	server.Auth = func(*http.Request) (Claims, bool) { return Claims{}, true }
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unpermissioned admin returned %d", w.Code)
	}
}

func TestAuthenticationAdministrationManagesSSOAndUsers(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("u", "password")
	if err != nil {
		t.Fatal(err)
	}
	oidc := NewOIDCService()
	admin := &AuthAdminService{Auth: auth, OIDC: oidc}
	server := Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"sso.manage": true, "users.manage": true, "auth.settings.manage": true}}, true
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/providers", bytes.NewBufferString(`{"key":"corp","issuer":"https://issuer.example","callback":"https://app.example/callback","enabled":true}`))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("provider create: %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/users/"+user.ID+"/disable", nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("disable: %d", w.Code)
	}
	if got, _ := auth.User(user.ID); got.Enabled {
		t.Fatal("disabled user remains enabled")
	}
}
