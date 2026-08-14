package api

import (
	"encoding/json"
	"net/http"
)

type AuthAdminService struct {
	Password *PasswordAuthService
	OIDC     *OIDCService
	Sessions *SessionManager
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
}
