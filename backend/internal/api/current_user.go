package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

type CurrentUserService struct {
	Profile  func(Claims) (map[string]any, error)
	Sessions *SessionManager
}

func (s Server) currentUserRoutes(mux routeRegistrar) {
	if s.CurrentUser == nil {
		return
	}
	mux.Handle("/api/v1/me", s.requireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := s.authenticator()(r)
		if r.Method == http.MethodGet {
			if s.CurrentUser.Profile == nil {
				writeJSON(w, 200, claims)
				return
			}
			profile, err := s.CurrentUser.Profile(claims)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "profile unavailable", err)
				return
			}
			writeJSON(w, 200, profile)
			return
		}
		if r.Method == http.MethodPut && s.AuthService != nil {
			var input struct {
				DisplayName string `json:"display_name"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeError(w, http.StatusBadRequest, "invalid profile request", err)
				return
			}
			if err := s.AuthService.UpdateProfile(claims.UserID, input.DisplayName); err != nil {
				writeError(w, http.StatusBadRequest, "profile update failed", err)
				return
			}
			profile, err := s.AuthService.Profile(claims)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "profile unavailable", err)
				return
			}
			writeJSON(w, http.StatusOK, profile)
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	})))
	mux.Handle("/api/v1/me/password", s.requireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || s.AuthService == nil {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		claims, _ := s.authenticator()(r)
		var input struct {
			CurrentPassword string `json:"current_password"`
			NewPassword     string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid password request", err)
			return
		}
		if err := s.AuthService.ChangePassword(claims.UserID, input.CurrentPassword, input.NewPassword); err != nil {
			writeError(w, http.StatusBadRequest, "password change failed", err)
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	})))
	mux.Handle("/api/v1/me/identities/", s.requireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || s.AuthService == nil {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		claims, _ := s.authenticator()(r)
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/me/identities/")
		if id == "" || s.AuthService.UnlinkOIDC(claims.UserID, id) != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "identity not found"})
			return
		}
		writeJSON(w, http.StatusNoContent, nil)
	})))
	mux.Handle("/api/v1/me/sessions/revoke", s.requireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		claims, _ := s.authenticator()(r)
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			sessionID = claims.SessionID
		}
		if s.CurrentUser.Sessions == nil || !s.CurrentUser.Sessions.Owns(claims.UserID, sessionID) {
			writeJSON(w, 403, map[string]string{"error": "session ownership required"})
			return
		}
		if s.CurrentUser.Sessions != nil {
			if err := s.CurrentUser.Sessions.Revoke(sessionID); err != nil {
				writeError(w, http.StatusInternalServerError, "session revoke failed", err)
				return
			}
		}
		writeJSON(w, 204, nil)
	})))
}
func (s Server) requireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := s.authenticator()
		if auth == nil {
			writeJSON(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		if _, ok := auth(r); !ok {
			writeJSON(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s Server) authenticator() Authenticator {
	if s.Auth != nil {
		return s.Auth
	}
	if s.AuthService != nil {
		return s.AuthService.Authenticator()
	}
	return nil
}
