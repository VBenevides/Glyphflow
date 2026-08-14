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
