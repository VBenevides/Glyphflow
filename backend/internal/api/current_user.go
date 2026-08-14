package api

import "net/http"

type CurrentUserService struct {
	Profile  func(Claims) map[string]any
	Sessions *SessionManager
}

func (s Server) currentUserRoutes(mux *http.ServeMux) {
	if s.CurrentUser == nil {
		return
	}
	mux.Handle("/api/v1/me", s.requireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, _ := s.Auth(r)
		if r.Method == http.MethodGet {
			if s.CurrentUser.Profile == nil {
				writeJSON(w, 200, claims)
				return
			}
			writeJSON(w, 200, s.CurrentUser.Profile(claims))
			return
		}
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
	})))
	mux.Handle("/api/v1/me/sessions/revoke", s.requireAuthenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		claims, _ := s.Auth(r)
		sessionID := r.URL.Query().Get("session_id")
		if sessionID == "" {
			sessionID = claims.SessionID
		}
		if sessionID != claims.SessionID {
			writeJSON(w, 403, map[string]string{"error": "session ownership required"})
			return
		}
		if s.CurrentUser.Sessions != nil {
			s.CurrentUser.Sessions.Revoke(sessionID)
		}
		writeJSON(w, 204, nil)
	})))
}
func (s Server) requireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Auth == nil {
			writeJSON(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		if _, ok := s.Auth(r); !ok {
			writeJSON(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
