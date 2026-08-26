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
			Enabled             *bool   `json:"enabled"`
			Registration        *bool   `json:"registration"`
			RequireUserApproval *bool   `json:"require_user_approval"`
			DefaultRoleID       *string `json:"default_role_id"`
			Lockdown            *bool   `json:"lockdown_scheduler"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid authentication settings request", err)
			return
		}
		before := map[string]any{}
		if s.AuthAdmin.Auth != nil {
			before = s.AuthAdmin.Auth.AuthSettings()
		} else if s.AuthAdmin.Password != nil {
			before = map[string]any{"passwordLoginEnabled": s.AuthAdmin.Password.Enabled(), "registration": s.AuthAdmin.Password.RegistrationEnabled(), "requireUserApproval": false, "defaultRoleId": ""}
		}
		enabled := false
		registration := false
		defaultRoleID := ""
		approvalRequired := false
		if s.AuthAdmin.Auth != nil {
			settings := s.AuthAdmin.Auth.AuthSettings()
			enabled, _ = settings["passwordLoginEnabled"].(bool)
			registration, _ = settings["registration"].(bool)
			defaultRoleID, _ = settings["defaultRoleId"].(string)
			approvalRequired, _ = settings["requireUserApproval"].(bool)
		} else if s.AuthAdmin.Password != nil {
			enabled = s.AuthAdmin.Password.Enabled()
			registration = s.AuthAdmin.Password.RegistrationEnabled()
		}
		if in.Enabled != nil {
			enabled = *in.Enabled
		}
		if in.Registration != nil {
			registration = *in.Registration
		}
		if in.RequireUserApproval != nil {
			approvalRequired = *in.RequireUserApproval
		}
		if in.DefaultRoleID != nil {
			defaultRoleID = *in.DefaultRoleID
		}
		authChanged := in.Enabled != nil || in.Registration != nil || in.RequireUserApproval != nil || in.DefaultRoleID != nil
		enabledSSO := 0
		if s.AuthAdmin.OIDC != nil {
			enabledSSO = s.AuthAdmin.OIDC.EnabledCount()
		}
		if authChanged && !enabled && !platform.HasLoginMethod(false, enabledSSO) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "a login method must remain enabled"})
			return
		}
		if s.AuthAdmin.Auth != nil {
			if authChanged {
				if err := s.AuthAdmin.Auth.UpdateAuthSettings(enabled, registration, defaultRoleID, approvalRequired); err != nil {
					writeError(w, http.StatusBadRequest, "authentication settings update failed", err)
					return
				}
			}
			if in.Lockdown != nil {
				if err := s.AuthAdmin.Auth.UpdateLockdownScheduler(*in.Lockdown); err != nil {
					writeError(w, http.StatusBadRequest, "general settings update failed", err)
					return
				}
			}
		}
		if s.AuthAdmin.Password != nil {
			s.AuthAdmin.Password.mu.Lock()
			if authChanged {
				s.AuthAdmin.Password.enabled = enabled
				s.AuthAdmin.Password.registration = registration
			}
			s.AuthAdmin.Password.mu.Unlock()
		}
		after := map[string]any{"passwordLoginEnabled": enabled, "registration": registration, "requireUserApproval": approvalRequired, "defaultRoleId": defaultRoleID}
		if s.AuthAdmin.Auth != nil {
			after = s.AuthAdmin.Auth.AuthSettings()
		}
		if s.AuditQuery != nil {
			claims, _ := s.authenticator()(r)
			actorName, actorEmail := s.auditActor(claims.UserID)
			s.AuditQuery.Add(AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: "success", CorrelationID: r.Header.Get("X-Correlation-ID"), Before: before, After: after, Input: auditInput(r), Output: map[string]any{"passwordLoginEnabled": enabled, "registrationEnabled": registration, "defaultRoleId": defaultRoleID}})
		}
		response := map[string]any{"password_login_enabled": enabled, "registration_enabled": registration, "require_user_approval": approvalRequired, "default_role_id": defaultRoleID}
		if s.AuthAdmin.Auth != nil {
			response["lockdown_scheduler"] = s.AuthAdmin.Auth.LockdownScheduler()
		}
		writeJSON(w, 200, response)
	})))
	mux.Handle("/api/v1/admin/auth/sessions/revoke", s.require("users.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || (s.AuthAdmin.Sessions == nil && s.AuthAdmin.Auth == nil) {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		if s.AuthAdmin.Auth != nil {
			s.AuthAdmin.Auth.Logout(r.URL.Query().Get("session_id"))
		} else {
			s.AuthAdmin.Sessions.Revoke(r.URL.Query().Get("session_id"))
		}
		writeJSON(w, 204, nil)
	})))
	mux.Handle("/api/v1/admin/auth/sessions", s.requireMethodRole(func(r *http.Request) string {
		return "users.read|users.manage"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || s.AuthAdmin.Auth == nil {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		sessions := s.AuthAdmin.Auth.AdminSessions()
		email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
		if email != "" {
			filtered := sessions[:0]
			for _, session := range sessions {
				if strings.Contains(strings.ToLower(session.UserEmail), email) {
					filtered = append(filtered, session)
				}
			}
			sessions = filtered
		}
		writePage(w, r, sessions)
	})))
	mux.Handle("/api/v1/admin/auth/providers", s.require("sso.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.AuthAdmin.OIDC == nil {
			writeJSON(w, 503, map[string]string{"error": "SSO unavailable"})
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, 200, s.AuthAdmin.OIDC.ConfiguredProviders())
		case http.MethodPost:
			var provider OIDCProvider
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&provider); err != nil {
				writeError(w, http.StatusBadRequest, "invalid SSO provider request", err)
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
			if err := s.AuthAdmin.OIDC.AddProvider(provider); err != nil {
				writeError(w, http.StatusBadRequest, "SSO provider update failed", err)
				return
			}
			writeJSON(w, 201, map[string]string{"key": provider.Key})
		default:
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		}
	})))
	mux.Handle("/api/v1/admin/auth/users/", s.require("users.manage", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.AuthAdmin.Auth == nil {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/auth/users/")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/disable"):
			userID := strings.TrimSuffix(path, "/disable")
			if err := s.AuthAdmin.Auth.DisableUser(userID); err != nil {
				if errors.Is(err, platform.ErrLastAdministrator) || errors.Is(err, platform.ErrSystemAdministrator) {
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, 404, map[string]string{"error": "user not found"})
				return
			}
			writeJSON(w, 204, nil)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/approve"):
			userID := strings.TrimSuffix(path, "/approve")
			if err := s.AuthAdmin.Auth.ApproveUser(userID); err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
					return
				}
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusNoContent, nil)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/roles"):
			userID := strings.TrimSuffix(path, "/roles")
			var input struct {
				Role string `json:"role"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Role) == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role is required"})
				return
			}
			if err := s.AuthAdmin.Auth.Grant(userID, input.Role); err != nil {
				if strings.Contains(err.Error(), "not found") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				} else {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				}
				return
			}
			writeJSON(w, http.StatusNoContent, nil)
		case r.Method == http.MethodDelete && strings.Contains(path, "/roles/"):
			parts := strings.SplitN(path, "/roles/", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user and role are required"})
				return
			}
			if err := s.AuthAdmin.Auth.Revoke(parts[0], parts[1]); err != nil {
				if errors.Is(err, platform.ErrLastAdministrator) || errors.Is(err, platform.ErrSystemAdministrator) {
					writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				} else if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not assigned") {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				} else {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				}
				return
			}
			writeJSON(w, http.StatusNoContent, nil)
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/sessions/revoke-all"):
			userID := strings.TrimSuffix(path, "/sessions/revoke-all")
			if _, ok := s.AuthAdmin.Auth.UserProfile(userID); !ok {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
				return
			}
			s.AuthAdmin.Auth.LogoutAll(userID)
			writeJSON(w, http.StatusNoContent, nil)
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
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
			users := s.AuthAdmin.Auth.Users()
			email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
			if email != "" {
				filtered := users[:0]
				for _, user := range users {
					userEmail, _ := user["email"].(string)
					if strings.Contains(strings.ToLower(userEmail), email) {
						filtered = append(filtered, user)
					}
				}
				users = filtered
			}
			writePage(w, r, users)
			return
		}
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var input passwordRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid user request", err)
			return
		}
		user, err := s.AuthAdmin.Auth.register(input.Email, input.Password, false)
		if err != nil {
			writeError(w, http.StatusBadRequest, "user creation failed", err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]string{"id": user.ID, "email": user.Email, "status": user.Status})
	})))
	mux.Handle("/api/v1/users/", s.requireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if s.AuthAdmin.Auth == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user details unavailable"})
			return
		}
		claims, _ := s.authenticator()(r)
		userID := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
		if userID == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		if userID != claims.UserID && !hasPermission(s.effectivePermissions(claims), "users.read|users.manage") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}
		profile, ok := s.AuthAdmin.Auth.UserProfile(userID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return
		}
		writeJSON(w, http.StatusOK, profile)
	})))
}
