package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type Claims struct {
	Subject   string
	UserID    string
	SessionID string
	Roles     map[string]bool
}
type Authenticator func(*http.Request) (Claims, bool)

type RuntimeConfig struct {
	Brand         string `json:"brand"`
	PasswordLogin bool   `json:"passwordLogin"`
	Registration  bool   `json:"registration"`
	OIDC          bool   `json:"oidc"`
	CSRFCookie    string `json:"csrfCookie"`
}

type Server struct {
	Auth            Authenticator
	Permissions     func(Claims) map[string]bool
	PasswordAuth    *PasswordAuthService
	AuthService     *AuthService
	Sessions        *SessionManager
	OIDC            *OIDCService
	AuthAdmin       *AuthAdminService
	Roles           *RoleAdminService
	CurrentUser     *CurrentUserService
	Audit           func(Claims, string, string)
	Ready           func(context.Context) error
	CSRFOrigin      string
	AuthRateLimiter *platform.RateLimiter
	Config          RuntimeConfig
	Operations      *OperationsService
	Runs            *RunService
}

func (s Server) Handler() http.Handler {
	if err := ValidateRouteRegistry(RouteRegistry()); err != nil {
		panic(err)
	}
	if s.AuthRateLimiter == nil {
		s.AuthRateLimiter = platform.NewRateLimiter(5, time.Minute)
	}
	if s.Operations == nil {
		s.Operations = NewOperationsService()
	}
	if s.Runs == nil {
		s.Runs = NewRunService()
	}
	mux := newTrackedMux()
	mux.HandleFunc("/docs", swaggerUI)
	mux.HandleFunc("/docs/login", s.docsLogin)
	mux.HandleFunc("/openapi.json", openAPI)
	mux.HandleFunc("/api/v1/config", s.runtimeConfig)
	if s.CurrentUser == nil && s.AuthService != nil {
		s.CurrentUser = &CurrentUserService{Profile: s.AuthService.Profile, Sessions: s.AuthService.sessions}
	}
	s.passwordRoutes(mux)
	s.oidcRoutes(mux)
	s.authAdminRoutes(mux)
	s.roleRoutes(mux)
	s.currentUserRoutes(mux)
	mux.HandleFunc("/api/v1/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/api/v1/readyz", func(w http.ResponseWriter, r *http.Request) {
		if s.Ready != nil {
			if err := s.Ready(r.Context()); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
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
		s.Operations.taskCollection(w, r)
	})))
	mux.Handle("/api/v1/schedules", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "tasks.read"
		}
		return "tasks.manage"
	}, http.HandlerFunc(s.Operations.scheduleCollection)))
	mux.Handle("/api/v1/schedules/preview", s.require("tasks.manage", http.HandlerFunc(s.Operations.preview)))
	for path, readPermission := range map[string]string{"/api/v1/resources": "resources.read", "/api/v1/users": "users.read", "/api/v1/roles": "roles.read", "/api/v1/sso": "sso.read", "/api/v1/logs": "logs.read"} {
		managePermission := map[string]string{"/api/v1/schedules": "tasks.manage", "/api/v1/resources": "resources.manage", "/api/v1/users": "users.manage", "/api/v1/roles": "roles.manage", "/api/v1/sso": "sso.manage", "/api/v1/logs": "logs.read"}[path]
		mux.Handle(path, s.requireMethodRole(func(r *http.Request) string {
			if r.Method == http.MethodGet {
				return readPermission
			}
			return managePermission
		}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "endpoint is not implemented"})
		})))
	}
	mux.Handle("/api/v1/runs", s.require("run.read", http.HandlerFunc(s.Runs.collection)))
	for path, role := range map[string]string{"/api/v1/events": "event.read", "/api/v1/runners": "runner.read", "/api/v1/audit": "audit.read"} {
		mux.Handle(path, s.require(role, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "endpoint is not implemented"})
		})))
	}
	mux.Handle("/api/v1/runs/execute", s.require("runs.execute", http.HandlerFunc(s.Runs.execute)))
	for path, permission := range map[string]string{"/api/v1/runs/retry": "runs.retry", "/api/v1/runs/cancel": "runs.cancel"} {
		mux.Handle(path, s.require(permission, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "run action is not implemented"})
		})))
	}
	mux.Handle("/api/v1/tasks/", s.requireMethodRole(func(r *http.Request) string {
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			return "run.cancel"
		}
		if strings.HasSuffix(r.URL.Path, "/retry") {
			return "run.retry"
		}
		if r.Method == http.MethodGet {
			return "task.read"
		}
		return "task.manage"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Operations.taskPath(w, r)
	})))
	mux.Handle("/api/v1/schedules/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "tasks.read"
		}
		return "tasks.manage"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Operations.schedulePath(w, r)
	})))
	mux.Handle("/api/v1/resources/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "resources.read"
		}
		return "resources.manage"
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "resource endpoint is not implemented"})
	})))
	mux.Handle("/api/v1/runners/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "runners.read"
		}
		return "runners.manage"
	}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "runner endpoint is not implemented"})
	})))
	mux.Handle("/api/v1/runs/", s.requireMethodRole(func(r *http.Request) string {
		if strings.Contains(r.URL.Path, "/logs") || strings.HasSuffix(r.URL.Path, "/events") {
			return "logs.read"
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			return "runs.cancel"
		}
		if strings.HasSuffix(r.URL.Path, "/retry") || strings.HasSuffix(r.URL.Path, "/reconcile") {
			return "runs.retry"
		}
		return "runs.read"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Runs.path(w, r)
	})))
	if err := ValidateBuiltRoutes(mux.patterns, RouteRegistry()); err != nil {
		panic(err)
	}
	var handler http.Handler = mux
	if s.CSRFOrigin != "" {
		handler = s.withCSRF(handler, s.CSRFOrigin)
	}
	return s.noStore(s.withCorrelation(handler))
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
		permissions := claims.Roles
		if s.Permissions != nil {
			permissions = s.Permissions(claims)
		}
		if !hasPermission(permissions, role) {
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		if s.Audit != nil {
			s.Audit(claims, r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func hasPermission(permissions map[string]bool, required string) bool {
	if permissions[required] {
		return true
	}
	aliases := map[string]string{"task.read": "tasks.read", "task.create": "tasks.manage", "run.read": "runs.read", "run.cancel": "runs.cancel", "run.retry": "runs.retry", "runner.read": "runners.read", "event.read": "logs.read"}
	return permissions[aliases[required]]
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
