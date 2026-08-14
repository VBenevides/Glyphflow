package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationAdministrationRequiresPermission(t *testing.T) {
	password := NewPasswordAuthService(true, false, nil)
	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	admin := &AuthAdminService{Password: password, OIDC: oidc}
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
	user, err := auth.Register("u", "correct horse")
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

func TestAuthenticationAdministrationPreventsLastLoginMethodRemoval(t *testing.T) {
	password := NewPasswordAuthService(true, false, nil)
	admin := &AuthAdminService{Password: password}
	server := Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"auth.settings.manage": true}}, true
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/settings", bytes.NewBufferString(`{"enabled":false}`))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("last password method removal returned %d", recorder.Code)
	}

	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	admin = &AuthAdminService{Password: NewPasswordAuthService(false, false, nil), OIDC: oidc}
	server = Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"sso.manage": true}}, true
	}}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/providers", bytes.NewBufferString(`{"key":"corp","issuer":"https://issuer.example","callback":"https://app.example/callback","enabled":false}`))
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("last SSO method removal returned %d", recorder.Code)
	}
}
