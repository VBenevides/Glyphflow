package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
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
	Before    map[string]any
	After     map[string]any
	Error     string
	Traceback string
}

const (
	auditErrorBodyLimit = 4 << 10
	maxRequestBodyBytes = 1 << 20

	permissionTaskRead           = "tasks.read"
	permissionTaskManage         = "tasks.manage"
	permissionTaskReadLegacy     = "task.read"
	permissionTaskCreate         = "task.create"
	permissionTaskManageLegacy   = "task.manage"
	permissionUsersManage        = "users.manage"
	permissionResourcesManage    = "resources.manage"
	permissionRunnersRead        = "runners.read"
	permissionRunnersManage      = "runners.manage"
	permissionLogsRead           = "logs.read"
	permissionRunsCancel         = "runs.cancel"
	permissionRunsRetry          = "runs.retry"
	headerCorrelationID          = "X-Correlation-ID"
	permissionUsersReadManage    = "users.read|users.manage"
	permissionTaskReadManage     = "tasks.read|tasks.manage"
	permissionRunnersReadManage  = "runners.read|runners.manage"
	permissionAuthSettingsManage = "auth.settings.manage"
	errorMethodNotAllowed        = "method not allowed"
	errorInvalidCredentials      = "invalid credentials"
	errorUserNotFound            = "user not found"
	errorNotFound                = "not found"
	errorTaskNotFound            = "task not found"
	errorScheduleNotFound        = "schedule not found"
	errorRunStorage              = "run storage unavailable"
	errorRunCreation             = "run creation failed"
	errorRunNotFound             = "run not found"
	errorRunActionNotAllowed     = "run action is not allowed in the current state"
	errorSecretNotFound          = "secret not found"
	errorSecretStatusUnavailable = "secret status unavailable"
	systemAdminSource            = "system-admin"
	errorGlobalVariableNotFound  = "global variable not found"
	errorRunnerPoolNotFound      = "runner pool not found"
	errorRunnerNotFound          = "runner not found"
	resourceIDPrefix             = "resource-"
	errorResourceStorage         = "resource storage unavailable"
	errorResourceNotFound        = "resource not found"
	errorDeadLetterStorage       = "dead-letter storage unavailable"
	errorDeadLetterNotFound      = "dead letter not found"
	errorDeadLetterRecovery      = "dead-letter recovery unavailable"
	headerContentType            = "Content-Type"
	authCookiePath               = "/api/v1/auth"
)

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
	Secrets                    *SecretAdminService
	SystemMetrics              *SystemMetricsService
	DeadLetters                *DeadLetterService
	ScheduleProjection         *controlplane.ProjectionService
	RequireDurableRepositories bool
}

func (s Server) ValidateDurableRepositories() error {
	if !s.RequireDurableRepositories {
		return nil
	}
	checks := []struct {
		valid   bool
		message string
	}{
		{s.Operations != nil && s.Operations.hasDurableRepositories(), "operations repositories are required"},
		{s.Runs != nil && s.Runs.hasDurableRepository(), "run repository is required"},
		{s.Infrastructure != nil && s.Infrastructure.hasDurableRepositories(), "infrastructure repositories are required"},
		{s.GlobalVariables != nil && s.GlobalVariables.hasDurableRepository(), "global variable repository is required"},
		{s.Secrets != nil && s.Secrets.hasDurableRepository(), "secret repository is required"},
		{s.AuditQuery != nil && s.AuditQuery.hasDurableRepository(), "audit repository is required"},
		{s.DeadLetters != nil && s.DeadLetters.repository != nil, "dead-letter repository is required"},
		{s.ExitCodes != nil, "exit-code repository is required"},
	}
	for _, check := range checks {
		if !check.valid {
			return errors.New(check.message)
		}
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
	s.initializeCoreServices()
	s.initializeAuthServices()
	mux := newTrackedMux()
	s.registerRoutes(mux)
	if err := ValidateBuiltRoutes(mux.patterns, RouteRegistry()); err != nil {
		panic(err)
	}
	return s.wrapHandler(mux)
}

func (s *Server) initializeCoreServices() {
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
	if s.Secrets == nil {
		s.Secrets = &SecretAdminService{}
	}
	if s.SystemMetrics == nil {
		s.SystemMetrics = NewSystemMetricsService(s.Metrics, s.Ready, s.Logger)
	}
	if s.DeadLetters == nil {
		s.DeadLetters = NewDeadLetterService(nil, nil)
	}
	if s.CurrentUser == nil && s.AuthService != nil {
		s.CurrentUser = &CurrentUserService{Profile: s.AuthService.Profile, Sessions: s.AuthService.sessions}
	}
}

func (s *Server) initializeAuthServices() {
	if s.AuthAdmin == nil && s.AuthService != nil {
		s.AuthAdmin = &AuthAdminService{Auth: s.AuthService, Sessions: s.AuthService.sessions, OIDC: s.OIDC}
	}
}

func (s Server) registerRoutes(mux *trackedMux) {
	mux.HandleFunc("/docs", swaggerUI)
	mux.HandleFunc("/docs/login", s.docsLogin)
	mux.HandleFunc("/openapi.json", openAPI)
	mux.HandleFunc("/api/v1/config", s.runtimeConfig)
	mux.Handle("/api/v1/runners/enroll", http.HandlerFunc(s.Infrastructure.enrollRunner))
	s.passwordRoutes(mux)
	s.oidcRoutes(mux)
	s.authAdminRoutes(mux)
	s.secretRoutes(mux)
	s.executionStatusRoutes(mux)
	s.roleRoutes(mux)
	s.currentUserRoutes(mux)
	s.registerHealthRoutes(mux)
	s.registerTaskRoutes(mux)
	s.registerInfrastructureRoutes(mux)
	s.registerDeprecatedRoutes(mux)
	s.registerPathRoutes(mux)
}

func (s Server) registerHealthRoutes(mux *trackedMux) {
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
}

func (s Server) registerTaskRoutes(mux *trackedMux) {
	mux.Handle("/api/v1/tasks", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodPost {
			return permissionTaskCreate
		}
		return permissionTaskReadLegacy
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		s.Operations.taskCollection(w, r)
	})))
	mux.Handle("/api/v1/schedules", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return permissionTaskRead
		}
		return permissionTaskManage
	}, http.HandlerFunc(s.Operations.scheduleCollection)))
	mux.Handle("/api/v1/schedule-projection", s.require(permissionTaskRead, http.HandlerFunc(s.scheduleProjection)))
	mux.Handle("/api/v1/schedules/preview", s.require(permissionTaskManage, http.HandlerFunc(s.Operations.preview)))
}

func (s Server) registerInfrastructureRoutes(mux *trackedMux) {
	mux.Handle("/api/v1/global-variables/options", s.require(permissionTaskRead, http.HandlerFunc(s.GlobalVariables.collection)))
	mux.Handle("/api/v1/global-variables", s.require(permissionUsersManage, http.HandlerFunc(s.GlobalVariables.collection)))
	mux.Handle("/api/v1/global-variables/", s.require(permissionUsersManage, http.HandlerFunc(s.GlobalVariables.path)))
	mux.Handle("/api/v1/resources", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "resources.read"
		}
		return permissionResourcesManage
	}, http.HandlerFunc(s.Infrastructure.resourceCollection)))
	mux.Handle("/api/v1/runners/pools", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return permissionRunnersRead
		}
		return permissionRunnersManage
	}, http.HandlerFunc(s.Infrastructure.poolCollection)))
	mux.Handle("/api/v1/runners/pools/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return permissionRunnersRead
		}
		return permissionRunnersManage
	}, http.HandlerFunc(s.Infrastructure.poolPath)))
	mux.Handle("/api/v1/runners", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return permissionRunnersRead
		}
		return permissionRunnersManage
	}, http.HandlerFunc(s.Infrastructure.runnerCollection)))
}

func (s Server) registerDeprecatedRoutes(mux *trackedMux) {
	for path, readPermission := range map[string]string{"/api/v1/roles": "roles.read", "/api/v1/sso": "sso.read", "/api/v1/logs": permissionLogsRead} {
		managePermission := map[string]string{"/api/v1/schedules": permissionTaskManage, "/api/v1/resources": permissionResourcesManage, "/api/v1/users": permissionUsersManage, "/api/v1/roles": "roles.manage", "/api/v1/sso": "sso.manage", "/api/v1/logs": permissionLogsRead}[path]
		mux.Handle(path, s.requireMethodRole(func(r *http.Request) string {
			if r.Method == http.MethodGet {
				return readPermission
			}
			return managePermission
		}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusGone, map[string]string{"error": "endpoint is deprecated; use the canonical administration or run endpoint"})
		})))
	}
}

func (s Server) registerPathRoutes(mux *trackedMux) {
	mux.Handle("/api/v1/runs", s.require("run.read", http.HandlerFunc(s.Runs.collection)))
	mux.Handle("/api/v1/audit", s.require("audit.read", http.HandlerFunc(s.AuditQuery.query)))
	mux.Handle("/api/v1/admin/system/metrics", s.require("system.metrics.read", http.HandlerFunc(s.SystemMetrics.metrics)))
	mux.Handle("/api/v1/admin/secrets/attention", s.require("secrets.read|secrets.manage", http.HandlerFunc(s.secretAttention)))
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
	for path, permission := range map[string]string{"/api/v1/runs/retry": permissionRunsRetry, "/api/v1/runs/cancel": permissionRunsCancel} {
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
			return permissionTaskReadLegacy
		}
		return permissionTaskManageLegacy
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Operations.taskPath(w, r)
	})))
	mux.Handle("/api/v1/schedules/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return permissionTaskRead
		}
		return permissionTaskManage
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Operations.schedulePath(w, r)
	})))
	mux.Handle("/api/v1/resources/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return "resources.read"
		}
		return permissionResourcesManage
	}, http.HandlerFunc(s.Infrastructure.resourcePath)))
	mux.Handle("/api/v1/runners/", s.requireMethodRole(func(r *http.Request) string {
		if r.Method == http.MethodGet {
			return permissionRunnersRead
		}
		return permissionRunnersManage
	}, http.HandlerFunc(s.Infrastructure.runnerPath)))
	mux.Handle("/api/v1/runs/", s.requireMethodRole(func(r *http.Request) string {
		if strings.Contains(r.URL.Path, "/logs") || strings.HasSuffix(r.URL.Path, "/events") {
			return permissionLogsRead
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			return permissionRunsCancel
		}
		if strings.HasSuffix(r.URL.Path, "/retry") || strings.HasSuffix(r.URL.Path, "/reconcile") {
			return permissionRunsRetry
		}
		return "runs.read"
	}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Runs.path(w, r)
	})))
}

func (s Server) wrapHandler(mux *trackedMux) http.Handler {
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
		claims, ok := s.authorize(w, r, role)
		if !ok {
			return
		}
		auditDetails := &requestAuditDetails{Input: captureAuditInput(r), Before: s.auditBefore(r)}
		if !s.recordAcceptedAudit(w, r, claims, auditDetails) {
			return
		}
		ctx := context.WithValue(r.Context(), requestClaimsContextKey{}, claims)
		ctx = context.WithValue(ctx, requestAuditContextKey{}, auditDetails)
		r = r.WithContext(ctx)
		if s.Audit != nil {
			s.Audit(claims, r.Method, r.URL.Path)
		}
		recorder := &auditResponseWriter{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		s.recordCompletedAudit(r, claims, auditDetails, recorder)
	})
}

func (s Server) authorize(w http.ResponseWriter, r *http.Request, role string) (Claims, bool) {
	if s.Auth == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return Claims{}, false
	}
	claims, ok := s.Auth(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return Claims{}, false
	}
	if hasPermission(s.effectivePermissions(claims), role) {
		return claims, true
	}
	if s.Metrics != nil {
		s.Metrics.PermissionDenials.Add(1)
	}
	if s.AuditQuery != nil {
		actorName, actorEmail := s.auditActor(claims.UserID)
		_ = s.AuditQuery.Add(AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: "failure", CorrelationID: r.Header.Get(headerCorrelationID), Output: map[string]any{"status": http.StatusForbidden, "error": "forbidden"}})
	}
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
	return Claims{}, false
}

func (s Server) recordAcceptedAudit(w http.ResponseWriter, r *http.Request, claims Claims, details *requestAuditDetails) bool {
	if !isMutatingMethod(r.Method) || s.AuditQuery == nil || !s.AuditQuery.hasDurableRepository() {
		return true
	}
	actorName, actorEmail := s.auditActor(claims.UserID)
	if err := s.AuditQuery.Add(AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: "accepted", CorrelationID: r.Header.Get(headerCorrelationID), Input: details.Input}); err != nil {
		writeError(w, http.StatusServiceUnavailable, "audit storage unavailable", err)
		return false
	}
	return true
}

func (s Server) recordCompletedAudit(r *http.Request, claims Claims, details *requestAuditDetails, recorder *auditResponseWriter) {
	if s.AuditQuery == nil {
		return
	}
	if recorder.status >= http.StatusBadRequest && details.Error == "" {
		details.Error = redactSensitiveText(auditResponseError(recorder.body, recorder.status))
		details.Traceback = string(debug.Stack())
	}
	result := "success"
	if recorder.status >= http.StatusBadRequest {
		result = "failure"
	}
	if result == "success" && isMutatingMethod(r.Method) {
		details.After = s.auditAfter(r, details, recorder)
	}
	if r.Method == http.MethodPost && r.URL.Path == "/api/v1/admin/auth/settings" && result == "success" {
		return
	}
	actorName, actorEmail := s.auditActor(claims.UserID)
	output := map[string]any{"status": recorder.status}
	if body := auditResponseBody(recorder.body); body != nil {
		output["body"] = body
	}
	traceback := ""
	if details.Error != "" {
		output["error"] = details.Error
		traceback = details.Error + "\n" + details.Traceback
	}
	event := AuditEvent{Actor: claims.UserID, ActorName: actorName, ActorEmail: actorEmail, Action: r.Method, Description: auditDescription(r.Method, r.URL.Path), Target: r.URL.Path, Result: result, CorrelationID: r.Header.Get(headerCorrelationID), Input: details.Input, Output: output, Before: details.Before, After: details.After, Traceback: traceback}
	if isLiveLogRequest(r) && result == "success" {
		_ = s.AuditQuery.AddLiveLog(liveLogAuditKey(claims, r), event)
		return
	}
	_ = s.AuditQuery.Add(event)
}

func (s Server) auditAfter(r *http.Request, details *requestAuditDetails, recorder *auditResponseWriter) map[string]any {
	if details.Before != nil {
		return s.auditBefore(r)
	}
	if body := auditResponseBody(recorder.body); body != nil {
		return auditSnapshot(body)
	}
	return nil
}

func auditSnapshot(value any) map[string]any {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"value": value}
	}
	var snapshot map[string]any
	if json.Unmarshal(raw, &snapshot) != nil {
		return map[string]any{"value": value}
	}
	return snapshot
}

func (s Server) auditBefore(r *http.Request) map[string]any {
	if !isMutatingMethod(r.Method) {
		return nil
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" {
		return nil
	}
	switch parts[2] {
	case "global-variables":
		return s.auditBeforeGlobalVariable(r, parts)
	case "tasks":
		return s.auditBeforeTask(r, parts)
	case "schedules":
		return s.auditBeforeSchedule(r, parts)
	case "resources":
		return s.auditBeforeResource(r, parts)
	case "runners":
		return s.auditBeforeRunner(r, parts)
	case "runs":
		return s.auditBeforeRun(r, parts)
	case "admin":
		return s.auditBeforeAdmin(r, parts)
	}
	return nil
}

func auditFind(value any, found bool) map[string]any {
	if !found {
		return nil
	}
	return auditSnapshot(value)
}

func (s Server) auditBeforeGlobalVariable(r *http.Request, parts []string) map[string]any {
	if len(parts) != 4 || parts[3] == "options" || s.GlobalVariables == nil {
		return nil
	}
	s.GlobalVariables.mu.RLock()
	repository := s.GlobalVariables.repository
	item, found := s.GlobalVariables.items[parts[3]]
	s.GlobalVariables.mu.RUnlock()
	if repository != nil {
		stored, ok, _ := repository.Find(r.Context(), parts[3])
		return auditFind(stored, ok)
	}
	return auditFind(item, found)
}

func (s Server) auditBeforeTask(r *http.Request, parts []string) map[string]any {
	if len(parts) < 4 || s.Operations == nil {
		return nil
	}
	s.Operations.mu.RLock()
	repository := s.Operations.repository
	s.Operations.mu.RUnlock()
	if repository != nil {
		stored, ok, _ := repository.Find(r.Context(), parts[3])
		return auditFind(taskRecordFromStore(stored), ok)
	}
	item, ok := s.Operations.task(parts[3])
	return auditFind(item, ok)
}

func (s Server) auditBeforeSchedule(r *http.Request, parts []string) map[string]any {
	if len(parts) < 4 || s.Operations == nil {
		return nil
	}
	s.Operations.mu.RLock()
	repository := s.Operations.scheduleRepository
	s.Operations.mu.RUnlock()
	if repository != nil {
		stored, ok, _ := repository.Find(r.Context(), parts[3])
		return auditFind(scheduleRecordFromStore(stored), ok)
	}
	item, ok := s.Operations.schedule(parts[3])
	return auditFind(item, ok)
}

func (s Server) auditBeforeResource(r *http.Request, parts []string) map[string]any {
	if len(parts) < 4 || s.Infrastructure == nil {
		return nil
	}
	s.Infrastructure.mu.RLock()
	repository := s.Infrastructure.resourceRepository
	item, found := s.Infrastructure.resources[parts[3]]
	s.Infrastructure.mu.RUnlock()
	if repository != nil {
		stored, ok, _ := repository.Find(r.Context(), parts[3])
		return auditFind(resourceRecordFromStore(stored), ok)
	}
	return auditFind(item, found)
}

func (s Server) auditBeforeRunner(r *http.Request, parts []string) map[string]any {
	if len(parts) < 4 || s.Infrastructure == nil {
		return nil
	}
	s.Infrastructure.mu.RLock()
	runnerRepository, poolRepository := s.Infrastructure.runnerRepository, s.Infrastructure.runnerRepository
	runner, runnerFound := s.Infrastructure.runners[parts[3]]
	pool, poolFound := s.Infrastructure.pools[parts[3]]
	s.Infrastructure.mu.RUnlock()
	if len(parts) >= 5 && parts[3] == "pools" {
		if poolRepository == nil {
			return auditFind(pool, poolFound)
		}
		stored, ok, _ := poolRepository.FindPool(r.Context(), parts[4])
		return auditFind(runnerPoolRecordFromStore(stored), ok)
	}
	if runnerRepository == nil {
		return auditFind(runner, runnerFound)
	}
	stored, ok, _ := runnerRepository.Find(r.Context(), parts[3])
	return auditFind(runnerRecordFromStore(stored), ok)
}

func (s Server) auditBeforeRun(r *http.Request, parts []string) map[string]any {
	if len(parts) < 4 || s.Runs == nil || parts[3] == "execute" {
		return nil
	}
	s.Runs.mu.RLock()
	repository := s.Runs.repository
	item, found := s.Runs.runs[parts[3]]
	s.Runs.mu.RUnlock()
	if repository != nil {
		stored, ok, _ := repository.Find(r.Context(), parts[3])
		return auditFind(runRecordFromStore(stored), ok)
	}
	return auditFind(item, found)
}

func (s Server) auditBeforeAdmin(r *http.Request, parts []string) map[string]any {
	if result := s.auditBeforeProvider(r, parts); result != nil {
		return result
	}
	if result := s.auditBeforeExitCode(r, parts); result != nil {
		return result
	}
	if result := s.auditBeforeRole(parts); result != nil {
		return result
	}
	if result := s.auditBeforeUser(parts); result != nil {
		return result
	}
	return s.auditBeforeDeadLetter(r, parts)
}

func (s Server) auditBeforeProvider(r *http.Request, parts []string) map[string]any {
	if len(parts) != 5 || parts[3] != "auth" || parts[4] != "providers" || s.AuthAdmin == nil || s.AuthAdmin.OIDC == nil {
		return nil
	}
	var input struct {
		Key string `json:"key"`
	}
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		_ = json.Unmarshal(raw, &input)
	}
	if input.Key == "" {
		return nil
	}
	provider, ok := s.AuthAdmin.OIDC.Provider(input.Key)
	return auditFind(provider, ok)
}

func (s Server) auditBeforeExitCode(r *http.Request, parts []string) map[string]any {
	if len(parts) != 5 || parts[3] != "execution-status" || s.ExitCodes == nil {
		return nil
	}
	code, err := strconv.Atoi(parts[4])
	if err != nil {
		return nil
	}
	items, err := s.ExitCodes.List(r.Context())
	if err != nil {
		return nil
	}
	for _, item := range items {
		if item.Code == code {
			return auditSnapshot(exitCodeRecords([]store.ExitCodeRecord{item})[0])
		}
	}
	return nil
}

func (s Server) auditBeforeRole(parts []string) map[string]any {
	if len(parts) < 5 || parts[3] != "roles" || s.Roles == nil {
		return nil
	}
	role, ok, _ := s.Roles.role(parts[4])
	if !ok {
		return nil
	}
	return auditSnapshot(map[string]any{"id": role.ID, "name": role.Name, "description": role.Description, "system": role.System, "permissions": role.Permissions, "assignedUsers": role.AssignedUsers})
}

func (s Server) auditBeforeUser(parts []string) map[string]any {
	if len(parts) < 6 || parts[3] != "auth" || parts[4] != "users" || s.AuthAdmin == nil || s.AuthAdmin.Auth == nil {
		return nil
	}
	user, ok, _ := s.AuthAdmin.Auth.UserProfile(parts[5])
	return auditFind(user, ok)
}

func (s Server) auditBeforeDeadLetter(r *http.Request, parts []string) map[string]any {
	if len(parts) < 5 || parts[3] != "dead-letters" || s.DeadLetters == nil || s.DeadLetters.repository == nil {
		return nil
	}
	item, ok, _ := s.DeadLetters.repository.Find(r.Context(), parts[4])
	return auditFind(deadLetterView(item), ok)
}

func isMutatingMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func isLiveLogRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/logs")
}

func liveLogAuditKey(claims Claims, r *http.Request) string {
	actor := claims.UserID
	if actor == "" {
		actor = claims.Subject
	}
	return actor + "\x00" + r.URL.Path + "\x00" + r.URL.Query().Get("stream")
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
	aliases := map[string]string{"task.read": "tasks.read", "task.create": permissionTaskManage, "task.manage": permissionTaskManage, "run.read": "runs.read", "run.cancel": permissionRunsCancel, "run.retry": permissionRunsRetry, "runner.read": "runners.read", "event.read": permissionLogsRead}
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
		w.Header().Set("Cache-Control", "no-store")
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
