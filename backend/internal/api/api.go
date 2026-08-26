package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type Claims struct {
	Subject   string
	UserID    string
	SessionID string
	Roles     map[string]bool
}
type requestClaimsContextKey struct{}
type requestAuditContextKey struct{}
type Authenticator func(*http.Request) (Claims, bool)

type requestAuditDetails struct {
	Input     map[string]any
	Error     string
	Traceback string
}

const auditErrorBodyLimit = 4 << 10
const maxRequestBodyBytes = 1 << 20

func recordRequestError(r *http.Request, err error) {
	if err == nil {
		return
	}
	details, ok := r.Context().Value(requestAuditContextKey{}).(*requestAuditDetails)
	if !ok {
		return
	}
	details.Error = redactSensitiveText(err.Error())
	details.Traceback = redactSensitiveText(string(debug.Stack()))
}

func recordRequestAuditField(r *http.Request, key string, value any) {
	details, ok := r.Context().Value(requestAuditContextKey{}).(*requestAuditDetails)
	if !ok || details.Input == nil || key == "" {
		return
	}
	details.Input[key] = value
}

type RuntimeConfig struct {
	Brand               string `json:"brand"`
	PasswordLogin       bool   `json:"passwordLogin"`
	Registration        bool   `json:"registration"`
	RequireUserApproval bool   `json:"requireUserApproval"`
	OIDC                bool   `json:"oidc"`
	LockdownScheduler   bool   `json:"lockdownScheduler"`
	CSRFCookie          string `json:"csrfCookie"`
	DefaultRoleID       string `json:"defaultRoleId,omitempty"`
}

type Server struct {
	Auth                       Authenticator
	Permissions                func(Claims) map[string]bool
	Metrics                    *platform.Metrics
	Logger                     *platform.Logger
	PasswordAuth               *PasswordAuthService
	AuthService                *AuthService
	Sessions                   *SessionManager
	OIDC                       *OIDCService
	AuthAdmin                  *AuthAdminService
	Roles                      *RoleAdminService
	CurrentUser                *CurrentUserService
	Audit                      func(Claims, string, string)
	Ready                      func(context.Context) error
	CSRFOrigin                 string
	CSRFOrigins                []string
	CORSOrigins                []string
	AuthRateLimiter            *platform.RateLimiter
	Config                     RuntimeConfig
	Operations                 *OperationsService
	Runs                       *RunService
	Infrastructure             *InfrastructureService
	AuditQuery                 *AuditQueryService
	ExitCodes                  store.ExitCodeRepository
	GlobalVariables            *GlobalVariableService
	SystemMetrics              *SystemMetricsService
	DeadLetters                *DeadLetterService
	RequireDurableRepositories bool
}

func (s Server) ValidateDurableRepositories() error {
	if !s.RequireDurableRepositories {
		return nil
	}
	if s.Operations == nil || !s.Operations.hasDurableRepositories() {
		return errors.New("operations repositories are required")
	}
	if s.Runs == nil || !s.Runs.hasDurableRepository() {
		return errors.New("run repository is required")
	}
	if s.Infrastructure == nil || !s.Infrastructure.hasDurableRepositories() {
		return errors.New("infrastructure repositories are required")
	}
	if s.GlobalVariables == nil || !s.GlobalVariables.hasDurableRepository() {
		return errors.New("global variable repository is required")
	}
	if s.AuditQuery == nil || !s.AuditQuery.hasDurableRepository() {
		return errors.New("audit repository is required")
	}
	if s.DeadLetters == nil || s.DeadLetters.repository == nil {
		return errors.New("dead-letter repository is required")
	}
	if s.ExitCodes == nil {
		return errors.New("exit-code repository is required")
	}
	return nil
}

func (s Server) Handler() http.Handler {
	if err := s.ValidateDurableRepositories(); err != nil {
		panic(err)
	}
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
	if s.Infrastructure == nil {
		s.Infrastructure = NewInfrastructureService()
	}
	if s.AuditQuery == nil {
		s.AuditQuery = NewAuditQueryService()
	}
	if s.GlobalVariables == nil {
		s.GlobalVariables = NewGlobalVariableService()
	}
	if s.SystemMetrics == nil {
		s.SystemMetrics = NewSystemMetricsService(s.Metrics, s.Ready, s.Logger)
	}
	if s.DeadLetters == nil {
		s.DeadLetters = NewDeadLetterService(nil, nil)
	}
	if s.AuthAdmin == nil && s.AuthService != nil {
		s.AuthAdmin = &AuthAdminService{Auth: s.AuthService, Sessions: s.AuthService.sessions, OIDC: s.OIDC}
	}
	mux := newTrackedMux()
	mux.HandleFunc("/docs", swaggerUI)
	mux.HandleFunc("/docs/login", s.docsLogin)
	mux.HandleFunc("/openapi.json", openAPI)
	mux.HandleFunc("/api/v1/config", s.runtimeConfig)
	mux.Handle("/api/v1/runners/enroll", http.HandlerFunc(s.Infrastructure.enrollRunner))
	if s.CurrentUser == nil && s.AuthService != nil {
		s.CurrentUser = &CurrentUserService{Profile: s.AuthService.Profile, Sessions: s.AuthService.sessions}
	}
	s.passwordRoutes(mux)
	s.oidcRoutes(mux)
	s.authAdminRoutes(mux)
	s.executionStatusRoutes(mux)
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
	mux.Handle("/api/v1/global-variables/options", s.require("tasks.read", http.HandlerFunc(s.GlobalVariables.collection)))
	mux.Handle("/api/v1/global-variables", s.require("users.manage", http.HandlerFunc(s.GlobalVariables.collection)))
	mux.Handle("/api/v1/global-variables/", s.require("users.manage", http.HandlerFunc(s.GlobalVariables.path)))
	mux.Handle("/api/v1/resources", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "resources.read"
		}
		return "resources.manage"
	}, http.HandlerFunc(s.Infrastructure.resourceCollection)))
	mux.Handle("/api/v1/runners/pools", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "runners.read"
		}
		return "runners.manage"
	}, http.HandlerFunc(s.Infrastructure.poolCollection)))
	mux.Handle("/api/v1/runners/pools/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "runners.read"
		}
		return "runners.manage"
	}, http.HandlerFunc(s.Infrastructure.poolPath)))
	mux.Handle("/api/v1/runners", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "runners.read"
		}
		return "runners.manage"
	}, http.HandlerFunc(s.Infrastructure.runnerCollection)))
	for path, readPermission := range map[string]string{"/api/v1/roles": "roles.read", "/api/v1/sso": "sso.read", "/api/v1/logs": "logs.read"} {
		managePermission := map[string]string{"/api/v1/schedules": "tasks.manage", "/api/v1/resources": "resources.manage", "/api/v1/users": "users.manage", "/api/v1/roles": "roles.manage", "/api/v1/sso": "sso.manage", "/api/v1/logs": "logs.read"}[path]
		mux.Handle(path, s.requireMethodRole(func(r *http.Request) string {
			if r.Method == http.MethodGet {
				return readPermission
			}
			return managePermission
		}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusGone, map[string]string{"error": "endpoint is deprecated; use the canonical administration or run endpoint"})
		})))
	}
	mux.Handle("/api/v1/runs", s.require("run.read", http.HandlerFunc(s.Runs.collection)))
	mux.Handle("/api/v1/audit", s.require("audit.read", http.HandlerFunc(s.AuditQuery.query)))
	mux.Handle("/api/v1/admin/system/metrics", s.require("system.metrics.read", http.HandlerFunc(s.SystemMetrics.metrics)))
	mux.Handle("/api/v1/admin/dead-letters", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "system.deadletter.read"
		}
		return "system.deadletter.manage"
	}, http.HandlerFunc(s.DeadLetters.collection)))
	mux.Handle("/api/v1/admin/dead-letters/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "system.deadletter.read"
		}
		return "system.deadletter.manage"
	}, http.HandlerFunc(s.DeadLetters.path)))
	for path, role := range map[string]string{"/api/v1/events": "event.read"} {
		mux.Handle(path, s.require(role, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusGone, map[string]string{"error": "endpoint is deprecated; use /api/v1/runs/{run_id}/events"})
		})))
	}
	mux.Handle("/api/v1/runs/execute", s.require("runs.execute", http.HandlerFunc(s.Runs.execute)))
	for path, permission := range map[string]string{"/api/v1/runs/retry": "runs.retry", "/api/v1/runs/cancel": "runs.cancel"} {
		mux.Handle(path, s.require(permission, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusGone, map[string]string{"error": "endpoint is deprecated; use /api/v1/runs/{run_id}/retry or /cancel"})
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
	}, http.HandlerFunc(s.Infrastructure.resourcePath)))
	mux.Handle("/api/v1/runners/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "runners.read"
		}
		return "runners.manage"
	}, http.HandlerFunc(s.Infrastructure.runnerPath)))
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
	handler = s.withLockdown(handler)
	csrfOrigins := s.CSRFOrigins
	if len(csrfOrigins) == 0 && s.CSRFOrigin != "" {
		csrfOrigins = []string{s.CSRFOrigin}
	}
	if len(csrfOrigins) > 0 {
		handler = s.withCSRF(handler, csrfOrigins)
	}
	if len(s.CORSOrigins) > 0 {
		handler = s.withCORS(handler)
	}
	return s.noStore(s.withCorrelation(s.limitRequestBody(handler)))
}

func (s Server) withLockdown(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.lockdownScheduler() && isWriteMethod(r.Method) && r.URL.Path != "/api/v1/admin/auth/settings" && !strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
			writeJSON(w, http.StatusLocked, map[string]string{"error": "scheduler is in lockdown: only read actions are allowed"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s Server) lockdownScheduler() bool {
	if s.AuthService != nil {
		return s.AuthService.LockdownScheduler()
	}
	return s.Config.LockdownScheduler
}

func isWriteMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete
}

func (s Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		allowed := false
		for _, configuredOrigin := range s.CORSOrigins {
			if configuredOrigin == "*" {
				continue
			}
			if configuredOrigin == requestOrigin {
				allowed = true
				break
			}
		}
		if !allowed {
			next.ServeHTTP(w, r)
			return
		}
		if len(s.CORSOrigins) > 1 {
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Origin", requestOrigin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s Server) limitRequestBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			if r.ContentLength > maxRequestBodyBytes {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body is too large"})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
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
		permissions := s.effectivePermissions(claims)
		if !hasPermission(permissions, role) {
			if s.Metrics != nil {
				s.Metrics.PermissionDenials.Add(1)
			}
			if s.AuditQuery != nil {
				actorName, actorEmail := s.auditActor(claims.UserID)
				_ = s.AuditQuery.Add(AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: "failure", CorrelationID: r.Header.Get("X-Correlation-ID"), Output: map[string]any{"status": http.StatusForbidden, "error": "forbidden"}})
			}
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		auditDetails := &requestAuditDetails{Input: captureAuditInput(r)}
		if isMutatingMethod(r.Method) && s.AuditQuery != nil && s.AuditQuery.hasDurableRepository() {
			actorName, actorEmail := s.auditActor(claims.UserID)
			if err := s.AuditQuery.Add(AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: "accepted", CorrelationID: r.Header.Get("X-Correlation-ID"), Input: auditDetails.Input}); err != nil {
				writeError(w, http.StatusServiceUnavailable, "audit storage unavailable", err)
				return
			}
		}
		ctx := context.WithValue(r.Context(), requestClaimsContextKey{}, claims)
		ctx = context.WithValue(ctx, requestAuditContextKey{}, auditDetails)
		r = r.WithContext(ctx)
		if s.Audit != nil {
			s.Audit(claims, r.Method, r.URL.Path)
		}
		recorder := &auditResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		if s.AuditQuery != nil {
			if recorder.status >= http.StatusBadRequest && auditDetails.Error == "" {
				auditDetails.Error = redactSensitiveText(auditResponseError(recorder.body, recorder.status))
				auditDetails.Traceback = string(debug.Stack())
			}
			actorName, actorEmail := s.auditActor(claims.UserID)
			result := "success"
			if recorder.status >= http.StatusBadRequest {
				result = "failure"
			}
			if !(r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/auth/settings" && result == "success") {
				output := map[string]any{"status": recorder.status}
				if body := auditResponseBody(recorder.body); body != nil {
					output["body"] = body
				}
				traceback := ""
				if auditDetails.Error != "" {
					output["error"] = auditDetails.Error
					traceback = auditDetails.Error + "\n" + auditDetails.Traceback
				}
				s.AuditQuery.Add(AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: result, CorrelationID: r.Header.Get("X-Correlation-ID"), Input: auditDetails.Input, Output: output, Traceback: traceback})
			}
		}
	})
}

func isMutatingMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func captureAuditInput(r *http.Request) map[string]any {
	var body any
	if r.Body != nil {
		raw, err := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		if err == nil && len(bytes.TrimSpace(raw)) > 0 {
			if json.Unmarshal(raw, &body) != nil {
				body = string(raw)
			}
		}
	}
	return map[string]any{"endpoint": r.URL.Path, "method": r.Method, "body": body}
}

func auditInput(r *http.Request) map[string]any {
	if details, ok := r.Context().Value(requestAuditContextKey{}).(*requestAuditDetails); ok && details.Input != nil {
		return details.Input
	}
	return captureAuditInput(r)
}

type auditResponseWriter struct {
	http.ResponseWriter
	status int
	body   []byte
}

func auditResponseError(body []byte, status int) string {
	var payload struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		if strings.TrimSpace(payload.Error) != "" {
			return payload.Error
		}
		if strings.TrimSpace(payload.Message) != "" {
			return payload.Message
		}
	}
	if value := strings.TrimSpace(string(body)); value != "" && len(value) <= auditErrorBodyLimit {
		return value
	}
	return http.StatusText(status)
}

func auditResponseBody(body []byte) any {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	var value any
	if json.Unmarshal(body, &value) == nil {
		return value
	}
	return string(body)
}

func (w *auditResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *auditResponseWriter) Write(value []byte) (int, error) {
	if len(w.body) < auditErrorBodyLimit {
		limit := auditErrorBodyLimit - len(w.body)
		if len(value) < limit {
			limit = len(value)
		}
		w.body = append(w.body, value[:limit]...)
	}
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(value)
}

func (s Server) auditActor(userID string) (string, string) {
	if s.AuthService == nil {
		return "", ""
	}
	user, ok := s.AuthService.User(userID)
	if !ok {
		return "", ""
	}
	return user.Username, user.Email
}

func (s Server) effectivePermissions(claims Claims) map[string]bool {
	if len(claims.Roles) > 0 {
		return claims.Roles
	}
	if s.Permissions != nil {
		return s.Permissions(claims)
	}
	if s.AuthService != nil {
		return s.AuthService.Permissions(claims)
	}
	return claims.Roles
}

func hasPermission(permissions map[string]bool, required string) bool {
	aliases := map[string]string{"task.read": "tasks.read", "task.create": "tasks.manage", "task.manage": "tasks.manage", "run.read": "runs.read", "run.cancel": "runs.cancel", "run.retry": "runs.retry", "runner.read": "runners.read", "event.read": "logs.read"}
	for _, candidate := range strings.Split(required, "|") {
		if permissions[candidate] || permissions[aliases[candidate]] {
			return true
		}
	}
	return false
}
func (s Server) requireMethodRole(role func(*http.Request) string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.require(role(r), next).ServeHTTP(w, r)
	})
}
func (s Server) withCorrelation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		correlationID := strings.TrimSpace(r.Header.Get("X-Correlation-ID"))
		if correlationID == "" {
			if generated, err := randomID(); err == nil {
				correlationID = generated
			} else {
				correlationID = time.Now().UTC().Format("20060102T150405.000000000Z")
			}
		}
		request := r.Clone(r.Context())
		request.Header.Set("X-Correlation-ID", correlationID)
		w.Header().Set("X-Correlation-ID", correlationID)
		next.ServeHTTP(w, request)
	})
}
func (s Server) noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "enrollment") || strings.HasSuffix(r.URL.Path, "/enroll") {
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

func writeError(w http.ResponseWriter, status int, operation string, err error) {
	// Do not reflect database, filesystem, or upstream provider details to clients.
	// Callers can record the original error with recordRequestError for correlated logs.
	writeJSON(w, status, map[string]string{"error": operation})
}
