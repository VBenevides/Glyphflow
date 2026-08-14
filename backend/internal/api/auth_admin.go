package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

type AuthAdminService struct {
	Password *PasswordAuthService
	OIDC     *OIDCService
	Sessions *SessionManager
	Auth     *AuthService
}

func (s Server) authAdminRoutes(mux *http.ServeMux) {
	if s.AuthAdmin == nil {
		return
	}
	mux.Handle("/api/v1/admin/auth/settings", s.require("auth.settings.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || s.AuthAdmin.Password == nil {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var in struct{ Enabled bool }
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid settings"})
			return
		}
		s.AuthAdmin.Password.mu.Lock()
		s.AuthAdmin.Password.enabled = in.Enabled
		s.AuthAdmin.Password.mu.Unlock()
		if s.AuthAdmin.Auth != nil {
			s.AuthAdmin.Auth.mu.Lock()
			s.AuthAdmin.Auth.passwordEnabled = in.Enabled
			s.AuthAdmin.Auth.mu.Unlock()
		}
		writeJSON(w, 200, map[string]bool{"password_login_enabled": in.Enabled})
	})))
	mux.Handle("/api/v1/admin/auth/sessions/revoke", s.require("users.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || s.AuthAdmin.Sessions == nil {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		s.AuthAdmin.Sessions.Revoke(r.URL.Query().Get("session_id"))
		writeJSON(w, 204, nil)
	})))
	mux.Handle("/api/v1/admin/auth/providers", s.require("sso.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.AuthAdmin.OIDC == nil {
			writeJSON(w, 503, map[string]string{"error": "SSO unavailable"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, s.AuthAdmin.OIDC.Providers())
		case http.MethodPost:
			var provider OIDCProvider
			if json.NewDecoder(r.Body).Decode(&provider) != nil || s.AuthAdmin.OIDC.AddProvider(provider) != nil {
				writeJSON(w, 400, map[string]string{"error": "provider update failed"})
				return
			}
			writeJSON(w, 201, map[string]string{"key": provider.Key})
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/admin/auth/users/", s.require("users.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || s.AuthAdmin.Auth == nil || !strings.HasSuffix(r.URL.Path, "/disable") {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		path := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/admin/auth/users/"), "/disable")
		if err := s.AuthAdmin.Auth.DisableUser(path); err != nil {
			writeJSON(w, 404, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, 204, nil)
	})))
}
