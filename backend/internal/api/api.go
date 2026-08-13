package api

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type Claims struct {
	Subject string
	Roles   map[string]bool
}
type Authenticator func(*http.Request) (Claims, bool)

func BearerAuthenticator(token string) Authenticator {
	return func(r *http.Request) (Claims, bool) {
		const prefix = "Bearer "
		value := r.Header.Get("Authorization")
		if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
			return Claims{}, false
		}
		if subtle.ConstantTimeCompare([]byte(value[len(prefix):]), []byte(token)) != 1 {
			return Claims{}, false
		}
		return Claims{Subject: "api-token", Roles: map[string]bool{"task.read": true, "task.create": true}}, true
	}
}

type Server struct {
	Auth  Authenticator
	Audit func(Claims, string, string)
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.Handle("/api/v1/tasks", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodPost {
			return "task.create"
		}
		return "task.read"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		if r.Method == http.MethodGet {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page < 1 {
				page = 1
			}
			writeJSON(w, 200, map[string]any{"items": []any{}, "page": page, "limit": 50})
			return
		}
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "task creation is not implemented"})
	})))
	return s.noStore(s.withCorrelation(mux))
}

func (s Server) require(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Auth == nil {
			writeJSON(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		claims, ok := s.Auth(r)
		if !ok {
			writeJSON(w, 401, map[string]string{"error": "authentication required"})
			return
		}
		if !claims.Roles[role] {
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		if s.Audit != nil {
			s.Audit(claims, r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
func (s Server) requireMethodRole(role func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.require(role(r), next).ServeHTTP(w, r)
	})
}
func (s Server) withCorrelation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Correlation-ID", r.Header.Get("X-Correlation-ID"))
		next.ServeHTTP(w, r)
	})
}
func (s Server) noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "enrollment") {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
