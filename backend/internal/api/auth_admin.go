package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type AuthAdminService struct {
	Password *PasswordAuthService
	OIDC     *OIDCService
	Sessions *SessionManager
	Auth     *AuthService
}

func userHasRoles(user map[string]any, required []string) bool {
	assigned, _ := user["roles"].([]string)
	for _, wanted := range required {
		found := false
		for _, role := range assigned {
			if strings.EqualFold(role, wanted) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (s Server) authAdminRoutes(mux routeRegistrar) {
	if s.AuthAdmin == nil {
		return
	}
	s.registerAuthSettingsRoute(mux)
	s.registerAuthSessionRoutes(mux)
	s.registerAuthProviderRoute(mux)
	s.registerAuthUserRoutes(mux)
	s.registerUsersRoute(mux)
	s.registerUserDetailsRoute(mux)
}

type authSettingsInput struct {
	Enabled             *bool   `json:"enabled"`
	Registration        *bool   `json:"registration"`
	RequireUserApproval *bool   `json:"require_user_approval"`
	DefaultRoleID       *string `json:"default_role_id"`
	Lockdown            *bool   `json:"lockdown_scheduler"`
}

func (s Server) registerAuthSettingsRoute(mux routeRegistrar) {
	mux.Handle("/api/v1/admin/auth/settings", s.require("auth.settings.manage", http.HandlerFunc(s.authSettingsHandler)))
}

func (s Server) authSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || (s.AuthAdmin.Password == nil && s.AuthAdmin.Auth == nil) {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	var input authSettingsInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid authentication settings request", err)
		return
	}
	before, enabled, registration, defaultRoleID, approvalRequired := s.authSettingsValues()
	enabled, registration, defaultRoleID, approvalRequired = applyAuthSettingsInput(input, enabled, registration, defaultRoleID, approvalRequired)
	authChanged := input.Enabled != nil || input.Registration != nil || input.RequireUserApproval != nil || input.DefaultRoleID != nil
	enabledSSO := 0
	if s.AuthAdmin.OIDC != nil {
		enabledSSO = s.AuthAdmin.OIDC.EnabledCount()
	}
	if authChanged && !enabled && !platform.HasLoginMethod(false, enabledSSO) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "a login method must remain enabled"})
		return
	}
	if message, err := s.persistAuthSettings(input, authChanged, enabled, registration, defaultRoleID, approvalRequired); err != nil {
		writeError(w, http.StatusBadRequest, message, err)
		return
	}
	after := map[string]any{"passwordLoginEnabled": enabled, "registration": registration, "requireUserApproval": approvalRequired, "defaultRoleId": defaultRoleID}
	if s.AuthAdmin.Auth != nil {
		after = s.AuthAdmin.Auth.AuthSettings()
	}
	s.auditAuthSettings(r, before, after, enabled, registration, defaultRoleID)
	response := map[string]any{"password_login_enabled": enabled, "registration_enabled": registration, "require_user_approval": approvalRequired, "default_role_id": defaultRoleID}
	if s.AuthAdmin.Auth != nil {
		response["lockdown_scheduler"] = s.AuthAdmin.Auth.LockdownScheduler()
	}
	writeJSON(w, http.StatusOK, response)
}

func (s Server) authSettingsValues() (map[string]any, bool, bool, string, bool) {
	before := map[string]any{}
	if s.AuthAdmin.Auth != nil {
		before = s.AuthAdmin.Auth.AuthSettings()
	} else if s.AuthAdmin.Password != nil {
		before = map[string]any{"passwordLoginEnabled": s.AuthAdmin.Password.Enabled(), "registration": s.AuthAdmin.Password.RegistrationEnabled(), "requireUserApproval": false, "defaultRoleId": ""}
	}
	enabled, registration, defaultRoleID, approvalRequired := false, false, "", false
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
	return before, enabled, registration, defaultRoleID, approvalRequired
}

func applyAuthSettingsInput(input authSettingsInput, enabled, registration bool, defaultRoleID string, approvalRequired bool) (bool, bool, string, bool) {
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	if input.Registration != nil {
		registration = *input.Registration
	}
	if input.RequireUserApproval != nil {
		approvalRequired = *input.RequireUserApproval
	}
	if input.DefaultRoleID != nil {
		defaultRoleID = *input.DefaultRoleID
	}
	return enabled, registration, defaultRoleID, approvalRequired
}

func (s Server) persistAuthSettings(input authSettingsInput, authChanged bool, enabled, registration bool, defaultRoleID string, approvalRequired bool) (string, error) {
	if s.AuthAdmin.Auth != nil {
		if authChanged {
			if err := s.AuthAdmin.Auth.UpdateAuthSettings(enabled, registration, defaultRoleID, approvalRequired); err != nil {
				return "authentication settings update failed", err
			}
		}
		if input.Lockdown != nil {
			if err := s.AuthAdmin.Auth.UpdateLockdownScheduler(*input.Lockdown); err != nil {
				return "general settings update failed", err
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
	return "", nil
}

func (s Server) auditAuthSettings(r *http.Request, before, after map[string]any, enabled, registration bool, defaultRoleID string) {
	if s.AuditQuery == nil {
		return
	}
	claims, _ := s.authenticator()(r)
	actorName, actorEmail := s.auditActor(claims.UserID)
	s.AuditQuery.Add(AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: "success", CorrelationID: r.Header.Get(headerCorrelationID), Before: before, After: after, Input: auditInput(r), Output: map[string]any{"passwordLoginEnabled": enabled, "registrationEnabled": registration, "defaultRoleId": defaultRoleID}})
}

func (s Server) registerAuthSessionRoutes(mux routeRegistrar) {
	mux.Handle("/api/v1/admin/auth/sessions/revoke", s.require(permissionUsersManage, http.HandlerFunc(s.revokeAuthSession)))
	mux.Handle("/api/v1/admin/auth/sessions", s.requireMethodRole(func(*http.Request) string { return permissionUsersReadManage }, http.HandlerFunc(s.listAuthSessions)))
}

func (s Server) revokeAuthSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || (s.AuthAdmin.Sessions == nil && s.AuthAdmin.Auth == nil) {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	var err error
	if s.AuthAdmin.Auth != nil {
		err = s.AuthAdmin.Auth.Logout(r.URL.Query().Get("session_id"))
	} else {
		err = s.AuthAdmin.Sessions.Revoke(r.URL.Query().Get("session_id"))
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session revoke failed", err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s Server) listAuthSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || s.AuthAdmin.Auth == nil {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	page, limit := collectionPage(r)
	if sessions, total, handled, err := s.AuthAdmin.Auth.AdminSessionsPage(email, limit, pageOffset(page, limit)); handled {
		if err != nil {
			writeError(w, http.StatusInternalServerError, "session list failed", err)
			return
		}
		writePageResult(w, page, limit, total, sessions)
		return
	}
	sessions, err := s.AuthAdmin.Auth.AdminSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "session list failed", err)
		return
	}
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
}

func (s Server) registerAuthProviderRoute(mux routeRegistrar) {
	mux.Handle("/api/v1/admin/auth/providers", s.require("sso.manage", http.HandlerFunc(s.authProviderHandler)))
}

func (s Server) authProviderHandler(w http.ResponseWriter, r *http.Request) {
	if s.AuthAdmin.OIDC == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "SSO unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.AuthAdmin.OIDC.ConfiguredProviders())
	case http.MethodPost:
		s.addAuthProvider(w, r)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
	}
}

func (s Server) addAuthProvider(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusCreated, map[string]string{"key": provider.Key})
}

func (s Server) registerAuthUserRoutes(mux routeRegistrar) {
	mux.Handle("/api/v1/admin/auth/users/", s.require(permissionUsersManage, http.HandlerFunc(s.authUserPathHandler)))
}

func (s Server) authUserPathHandler(w http.ResponseWriter, r *http.Request) {
	if s.AuthAdmin.Auth == nil {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/admin/auth/users/")
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/disable"):
		s.disableAdminUser(w, strings.TrimSuffix(path, "/disable"))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/approve"):
		s.approveAdminUser(w, strings.TrimSuffix(path, "/approve"))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/roles"):
		s.grantAdminUserRole(w, r, strings.TrimSuffix(path, "/roles"))
	case r.Method == http.MethodDelete && strings.Contains(path, "/roles/"):
		s.revokeAdminUserRole(w, path)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/sessions/revoke-all"):
		s.revokeAllAdminUserSessions(w, strings.TrimSuffix(path, "/sessions/revoke-all"))
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
	}
}

func (s Server) disableAdminUser(w http.ResponseWriter, userID string) {
	if err := s.AuthAdmin.Auth.DisableUser(userID); err != nil {
		if errors.Is(err, platform.ErrLastAdministrator) || errors.Is(err, platform.ErrSystemAdministrator) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorUserNotFound})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s Server) approveAdminUser(w http.ResponseWriter, userID string) {
	if err := s.AuthAdmin.Auth.ApproveUser(userID); err != nil {
		if strings.Contains(err.Error(), errorNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": errorUserNotFound})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s Server) grantAdminUserRole(w http.ResponseWriter, r *http.Request, userID string) {
	var input struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Role) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role is required"})
		return
	}
	if err := s.AuthAdmin.Auth.Grant(userID, input.Role); err != nil {
		if strings.Contains(err.Error(), errorNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s Server) revokeAdminUserRole(w http.ResponseWriter, path string) {
	parts := strings.SplitN(path, "/roles/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user and role are required"})
		return
	}
	if err := s.AuthAdmin.Auth.Revoke(parts[0], parts[1]); err != nil {
		if errors.Is(err, platform.ErrLastAdministrator) || errors.Is(err, platform.ErrSystemAdministrator) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		} else if strings.Contains(err.Error(), errorNotFound) || strings.Contains(err.Error(), "not assigned") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s Server) revokeAllAdminUserSessions(w http.ResponseWriter, userID string) {
	if _, ok := s.AuthAdmin.Auth.User(userID); !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorUserNotFound})
		return
	}
	if err := s.AuthAdmin.Auth.LogoutAll(userID); err != nil {
		writeError(w, http.StatusInternalServerError, "session revoke-all failed", err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s Server) registerUsersRoute(mux routeRegistrar) {
	mux.Handle("/api/v1/users", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return permissionUsersReadManage
		}
		return permissionUsersManage
	}, http.HandlerFunc(s.usersHandler)))
}

func (s Server) usersHandler(w http.ResponseWriter, r *http.Request) {
	if s.AuthAdmin.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user administration unavailable"})
		return
	}
	if r.Method == http.MethodGet {
		s.listUsers(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	s.createAdminUser(w, r)
}

func (s Server) listUsers(w http.ResponseWriter, r *http.Request) {
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && !store.ValidUserStatus(status) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user status"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	roles := uniqueStrings(strings.Split(strings.ToLower(r.URL.Query().Get("roles")), ","))
	page, limit := collectionPage(r)
	if users, total, handled, err := s.AuthAdmin.Auth.UsersPage(status, email, roles, limit, pageOffset(page, limit)); handled {
		if err != nil {
			writeError(w, http.StatusInternalServerError, "user list failed", err)
			return
		}
		writePageResult(w, page, limit, total, users)
		return
	}
	users, err := s.AuthAdmin.Auth.Users(status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user list failed", err)
		return
	}
	writePage(w, r, filterUsers(users, email, roles))
}

func filterUsers(users []map[string]any, email string, roles []string) []map[string]any {
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
	if len(roles) > 0 {
		filtered := users[:0]
		for _, user := range users {
			if userHasRoles(user, roles) {
				filtered = append(filtered, user)
			}
		}
		users = filtered
	}
	return users
}

func (s Server) createAdminUser(w http.ResponseWriter, r *http.Request) {
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
}

func (s Server) registerUserDetailsRoute(mux routeRegistrar) {
	mux.Handle("/api/v1/users/", s.requireAuthenticated(http.HandlerFunc(s.userDetailsHandler)))
}

func (s Server) userDetailsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": errorMethodNotAllowed})
		return
	}
	if s.AuthAdmin.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "user details unavailable"})
		return
	}
	claims, _ := s.authenticator()(r)
	userID := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if userID == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorUserNotFound})
		return
	}
	if userID != claims.UserID && !hasPermission(s.effectivePermissions(claims), permissionUsersReadManage) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}
	profile, ok, err := s.AuthAdmin.Auth.UserProfile(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user details unavailable", err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": errorUserNotFound})
		return
	}
	writeJSON(w, http.StatusOK, profile)
}
