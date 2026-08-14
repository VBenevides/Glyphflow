package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type AuthAdminService struct {
	Password *PasswordAuthService
	OIDC     *OIDCService
	Sessions *SessionManager
	Auth     *AuthService
}

func (s Server) authAdminRoutes(mux routeRegistrar) {
	if s.AuthAdmin == nil {
		return
	}
	mux.Handle("/api/v1/admin/auth/settings", s.require("auth.settings.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || (s.AuthAdmin.Password == nil && s.AuthAdmin.Auth == nil) {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		var in struct {
			Enabled      bool   `json:"enabled"`
			Registration bool   `json:"registration"`
			DefaultRole  string `json:"default_role"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid settings"})
			return
		}
		enabledSSO := 0
		if s.AuthAdmin.OIDC != nil {
			enabledSSO = s.AuthAdmin.OIDC.EnabledCount()
		}
		if !in.Enabled && !platform.HasLoginMethod(false, enabledSSO) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a login method must remain enabled"})
			return
		}
		if s.AuthAdmin.Password != nil {
			s.AuthAdmin.Password.mu.Lock()
			s.AuthAdmin.Password.enabled = in.Enabled
			s.AuthAdmin.Password.registration = in.Registration
			s.AuthAdmin.Password.mu.Unlock()
		}
		if s.AuthAdmin.Auth != nil {
			if err := s.AuthAdmin.Auth.UpdateAuthSettings(in.Enabled, in.Registration, in.DefaultRole); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid authentication settings"})
				return
			}
		}
		writeJSON(w, 200, map[string]any{"password_login_enabled": in.Enabled, "registration_enabled": in.Registration, "default_role": in.DefaultRole})
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
			if json.NewDecoder(r.Body).Decode(&provider) != nil {
				writeJSON(w, 400, map[string]string{"error": "provider update failed"})
				return
			}
			previous, existed := s.AuthAdmin.OIDC.Provider(provider.Key)
			passwordEnabled := false
			if s.AuthAdmin.Password != nil {
				passwordEnabled = s.AuthAdmin.Password.Enabled()
			} else if s.AuthAdmin.Auth != nil {
				passwordEnabled = s.AuthAdmin.Auth.PasswordLoginEnabled()
			}
			if existed && previous.Enabled && !provider.Enabled && !passwordEnabled && s.AuthAdmin.OIDC.EnabledCount() == 1 {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "a login method must remain enabled"})
				return
			}
			if s.AuthAdmin.OIDC.AddProvider(provider) != nil {
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
			if errors.Is(err, platform.ErrLastAdministrator) || errors.Is(err, platform.ErrSystemAdministrator) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, 404, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, 204, nil)
	})))
	mux.Handle("/api/v1/users", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "users.read|users.manage"
		}
		return "users.manage"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.AuthAdmin.Auth == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user administration unavailable"})
			return
		}
		if r.Method == http.MethodGet {
			writePage(w, r, s.AuthAdmin.Auth.Users())
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var input passwordRequest
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user creation failed"})
			return
		}
		user, err := s.AuthAdmin.Auth.Register(input.Email, input.Password)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user creation failed"})
			return
		}
		writeJSON(w, http.StatusCreated, user)
	})))
}
