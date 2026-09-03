package api

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

func TestHandlerCoverageAcrossConfiguredRoutes(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	permissions := map[string]bool{}
	for _, permission := range platform.PermissionCatalog {
		permissions[permission] = true
	}
	for _, permission := range []string{"task.read", "task.create", "task.manage", "run.read", "run.cancel", "run.retry", "runner.read", "event.read"} {
		permissions[permission] = true
	}
	server := Server{
		Auth: func(*http.Request) (Claims, bool) {
			return Claims{UserID: "coverage-user", SessionID: "coverage-session"}, true
		},
		Permissions:        func(Claims) map[string]bool { return permissions },
		AuthService:        auth,
		OIDC:               NewOIDCService(),
		Roles:              NewRoleAdminService(),
		Infrastructure:     NewInfrastructureService(),
		Operations:         NewOperationsService(),
		Runs:               NewRunService(),
		GlobalVariables:    NewGlobalVariableService(),
		Secrets:            NewSecretAdminService(nil, nil),
		SystemMetrics:      NewSystemMetricsService(nil, func(context.Context) error { return nil }, nil),
		DeadLetters:        NewDeadLetterService(nil, nil),
		ExitCodes:          &testExitCodes{},
		ScheduleProjection: controlplane.NewProjectionService(nil, nil),
	}
	handler := server.Handler()
	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/docs", ""}, {http.MethodGet, "/docs/login", ""}, {http.MethodGet, "/openapi.json", ""},
		{http.MethodGet, "/api/v1/config", ""}, {http.MethodGet, "/api/v1/healthz", ""}, {http.MethodGet, "/api/v1/readyz", ""},
		{http.MethodGet, "/api/v1/auth/oidc/providers", ""}, {http.MethodGet, "/api/v1/me", ""},
		{http.MethodGet, "/api/v1/tasks", ""}, {http.MethodGet, "/api/v1/tasks/missing", ""},
		{http.MethodGet, "/api/v1/schedules", ""}, {http.MethodGet, "/api/v1/schedules/missing", ""},
		{http.MethodGet, "/api/v1/schedule-projection", ""}, {http.MethodGet, "/api/v1/resources", ""},
		{http.MethodGet, "/api/v1/resources/missing", ""}, {http.MethodGet, "/api/v1/runners", ""},
		{http.MethodGet, "/api/v1/runners/pools", ""}, {http.MethodGet, "/api/v1/runners/pools/missing", ""},
		{http.MethodGet, "/api/v1/runs", ""}, {http.MethodGet, "/api/v1/runs/missing", ""},
		{http.MethodGet, "/api/v1/users", ""}, {http.MethodGet, "/api/v1/users/missing", ""},
		{http.MethodGet, "/api/v1/admin/roles", ""}, {http.MethodGet, "/api/v1/admin/roles/missing", ""},
		{http.MethodGet, "/api/v1/admin/auth/sessions", ""}, {http.MethodGet, "/api/v1/admin/auth/providers", ""},
		{http.MethodGet, "/api/v1/admin/execution-status", ""}, {http.MethodGet, "/api/v1/admin/execution-status/nope", ""},
		{http.MethodGet, "/api/v1/admin/secrets", ""}, {http.MethodGet, "/api/v1/admin/secrets/missing", ""},
		{http.MethodGet, "/api/v1/admin/dead-letters", ""}, {http.MethodGet, "/api/v1/admin/dead-letters/missing", ""},
		{http.MethodGet, "/api/v1/admin/system/metrics", ""}, {http.MethodGet, "/api/v1/audit", ""},
		{http.MethodGet, "/api/v1/roles", ""}, {http.MethodGet, "/api/v1/sso", ""}, {http.MethodGet, "/api/v1/logs", ""},
		{http.MethodPost, "/api/v1/auth/register", `{}`}, {http.MethodPost, "/api/v1/auth/login", `{}`},
		{http.MethodPost, "/api/v1/auth/logout", ""}, {http.MethodPost, "/api/v1/auth/refresh", `{}`},
		{http.MethodPost, "/api/v1/admin/auth/settings", `{}`}, {http.MethodPost, "/api/v1/admin/auth/sessions/revoke", ""},
		{http.MethodPost, "/api/v1/admin/auth/providers", `{}`}, {http.MethodPost, "/api/v1/users", `{}`},
		{http.MethodPost, "/api/v1/admin/roles", `{}`}, {http.MethodPut, "/api/v1/admin/roles/missing", `{}`},
		{http.MethodDelete, "/api/v1/admin/roles/missing", ""}, {http.MethodPost, "/api/v1/tasks", `{}`},
		{http.MethodPost, "/api/v1/schedules", `{}`}, {http.MethodPost, "/api/v1/schedules/preview", `{}`},
		{http.MethodPost, "/api/v1/global-variables", `{}`}, {http.MethodPost, "/api/v1/resources", `{}`},
		{http.MethodPost, "/api/v1/runners", `{}`}, {http.MethodPost, "/api/v1/runs/execute", `{}`},
		{http.MethodPost, "/api/v1/runs/missing/cancel", `{}`}, {http.MethodPost, "/api/v1/runs/missing/retry", `{}`},
		{http.MethodPost, "/api/v1/admin/dead-letters/missing/retry", `{}`}, {http.MethodPost, "/api/v1/admin/dead-letters/missing/reconcile", `{}`},
		{http.MethodPost, "/api/v1/admin/execution-status", `{}`}, {http.MethodPut, "/api/v1/admin/execution-status/1", `{}`},
		{http.MethodDelete, "/api/v1/admin/execution-status/1", ""}, {http.MethodPost, "/api/v1/admin/auth/users/missing/approve", ""},
		{http.MethodPost, "/api/v1/admin/auth/users/missing/disable", ""}, {http.MethodPost, "/api/v1/admin/auth/users/missing/roles", `{}`},
		{http.MethodDelete, "/api/v1/admin/auth/users/missing/roles/missing", ""}, {http.MethodPost, "/api/v1/admin/auth/users/missing/sessions/revoke-all", ""},
		{http.MethodPut, "/api/v1/me", `{}`}, {http.MethodPost, "/api/v1/me/password", `{}`},
		{http.MethodDelete, "/api/v1/me/identities/missing", ""}, {http.MethodPost, "/api/v1/me/sessions/revoke", ""},
		{http.MethodPut, "/api/v1/global-variables/missing", `{}`}, {http.MethodDelete, "/api/v1/global-variables/missing", ""},
		{http.MethodPut, "/api/v1/resources/missing", `{}`}, {http.MethodDelete, "/api/v1/resources/missing", ""},
		{http.MethodPut, "/api/v1/runners/missing", `{}`}, {http.MethodDelete, "/api/v1/runners/missing", ""},
		{http.MethodPut, "/api/v1/schedules/missing", `{}`}, {http.MethodDelete, "/api/v1/schedules/missing", ""},
		{http.MethodDelete, "/api/v1/tasks/missing", ""}, {http.MethodPost, "/api/v1/tasks/missing/versions", `{}`},
		{http.MethodGet, "/api/v1/runs/missing/events", ""}, {http.MethodGet, "/api/v1/runs/missing/logs", ""},
	}
	for _, item := range requests {
		t.Run(item.method+" "+item.path, func(t *testing.T) {
			request := httptest.NewRequest(item.method, item.path, strings.NewReader(item.body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code == 0 {
				t.Fatal("handler did not write a response")
			}
		})
	}

	if _, err := NewAuthService("short", true, true, nil); err == nil {
		t.Fatal("short session secret accepted")
	}
	password := NewPasswordAuthService(true, true, nil)
	if err := password.Register("coverage@example.com", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if !password.Verify("coverage@example.com", "correct horse") || password.Verify("coverage@example.com", "wrong") {
		t.Fatal("password verification failed")
	}
	if err := password.Register("coverage@example.com", "correct horse"); err == nil {
		t.Fatal("duplicate password user accepted")
	}
}

func TestInMemoryOperationsCoverage(t *testing.T) {
	o := NewOperationsService()
	create := httptest.NewRecorder()
	o.taskCollection(create, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"name":"Build","command":["echo","ok"],"runner_pool":"default","duration_seconds":30}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("create task status = %d", create.Code)
	}
	taskID := "task-1"
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks?search=build&state=enabled", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID, nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+taskID+"/versions", nil),
	} {
		response := httptest.NewRecorder()
		if request.URL.Path == "/api/v1/tasks" {
			o.taskCollection(response, request)
		} else {
			o.taskPath(response, request)
		}
	}
	version := httptest.NewRecorder()
	o.taskPath(version, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+taskID+"/versions", strings.NewReader(`{"command":["echo","new"],"runner_pool":"new-pool"}`)))
	if version.Code != http.StatusCreated {
		t.Fatalf("create version status = %d", version.Code)
	}
	scheduleBody := `{"name":"Every minute","task_id":"task-1","expression":"* * * * *","timezone":"UTC"}`
	schedule := httptest.NewRecorder()
	o.scheduleCollection(schedule, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(scheduleBody)))
	if schedule.Code != http.StatusCreated {
		t.Fatalf("create schedule status = %d body=%s", schedule.Code, schedule.Body.String())
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/schedules?task=task-1&enabled=true", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/schedules/schedule-1", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/schedules/schedule-1/disable", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/schedules/schedule-1/enable", nil),
		httptest.NewRequest(http.MethodPut, "/api/v1/schedules/schedule-1", strings.NewReader(scheduleBody)),
	} {
		response := httptest.NewRecorder()
		if request.URL.Path == "/api/v1/schedules" {
			o.scheduleCollection(response, request)
		} else {
			o.schedulePath(response, request)
		}
	}
	deleteSchedule := httptest.NewRecorder()
	o.schedulePath(deleteSchedule, httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/schedule-1", nil))
	deleteTask := httptest.NewRecorder()
	o.taskPath(deleteTask, httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/"+taskID, nil))
	if deleteTask.Code != http.StatusNoContent || deleteSchedule.Code != http.StatusNoContent {
		t.Fatalf("delete statuses task=%d schedule=%d", deleteTask.Code, deleteSchedule.Code)
	}
	if _, ok := o.task(taskID); ok || o.deleteTask(taskID) {
		t.Fatal("deleted task remained active")
	}
	if o.deleteSchedule("missing") {
		t.Fatal("missing schedule deleted")
	}
}

func TestInMemoryInfrastructureCoverage(t *testing.T) {
	s := NewInfrastructureService()
	poolResponse := httptest.NewRecorder()
	s.poolCollection(poolResponse, httptest.NewRequest(http.MethodPost, "/api/v1/runners/pools", strings.NewReader(`{"name":"Extra","description":"pool"}`)))
	var pool RunnerPoolRecord
	if err := json.Unmarshal(poolResponse.Body.Bytes(), &pool); err != nil {
		t.Fatal(err)
	}
	s.poolPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runners/pools/"+pool.ID, nil))
	s.poolPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/api/v1/runners/pools/"+pool.ID, strings.NewReader(`{"name":"Extra 2","enabled":false}`)))
	s.poolPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v1/runners/pools/"+pool.ID, nil))

	resourceResponse := httptest.NewRecorder()
	s.resourceCollection(resourceResponse, httptest.NewRequest(http.MethodPost, "/api/v1/resources", strings.NewReader(`{"name":"Database","kind":"exclusive"}`)))
	var resource ResourceRecord
	if err := json.Unmarshal(resourceResponse.Body.Bytes(), &resource); err != nil {
		t.Fatal(err)
	}
	s.resourceCollection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/resources?search=database&kind=exclusive", nil))
	leaseResponse := httptest.NewRecorder()
	s.resourcePath(leaseResponse, httptest.NewRequest(http.MethodPost, "/api/v1/resources/"+resource.ID+"/lease", strings.NewReader(`{"holder":"runner","ttl_seconds":30}`)))
	var lease ResourceRecord
	if err := json.Unmarshal(leaseResponse.Body.Bytes(), &lease); err != nil {
		t.Fatal(err)
	}
	s.resourcePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/resources/"+resource.ID, nil))
	release := httptest.NewRequest(http.MethodDelete, "/api/v1/resources/"+resource.ID+"/lease", strings.NewReader(`{"holder":"runner","fencing_token":1}`))
	s.resourcePath(httptest.NewRecorder(), release)
	s.resourcePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v1/resources/"+resource.ID, nil))

	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "glyphflow-runner-linux-amd64-headless"), []byte("runner"), 0o700); err != nil {
		t.Fatal(err)
	}
	s.SetRunnerBinaryDirectory(directory)
	s.SetRunnerArtifactConfig("nats://localhost:4222", 1024)
	s.SetRunnerControlPlaneURL("http://control.example")
	s.SetControlPlanePublicKey(base64.RawStdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)))
	enrollResponse := httptest.NewRecorder()
	s.enroll(enrollResponse, httptest.NewRequest(http.MethodPost, "/api/v1/runners/enrollments", strings.NewReader(`{"runner_id":"runner-coverage","platform":"linux","architecture":"amd64","ui":"headless","capacity":2}`)))
	if enrollResponse.Code != http.StatusCreated {
		t.Fatalf("enrollment status = %d body=%s", enrollResponse.Code, enrollResponse.Body.String())
	}
	s.runnerCollection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runners?search=runner-coverage&desired_state=ENABLED", nil))
	s.runnerPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runners/runner-coverage", nil))
	controlKey, err := protocol.GenerateSigningKey("control", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s.SetRunnerCapacityPublisher(queue.NewMemory(), controlKey)
	s.runnerPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/api/v1/runners/runner-coverage", strings.NewReader(`{"capacity":3,"nats_endpoint":"nats://new","control_plane_url":"http://new.example/"}`)))
	for _, action := range []string{"enable", "disable", "drain", "reset", "revoke"} {
		s.runnerPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/runners/runner-coverage/"+action, nil))
	}
	s.runnerPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runners/runner-coverage/metrics", nil))
	s.runnerPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v1/runners/runner-coverage", nil))
	if remoteAddress(httptest.NewRequest(http.MethodGet, "/", nil)) == "" || requestBaseURL(httptest.NewRequest(http.MethodGet, "/", nil)) == "" {
		t.Fatal("request metadata helpers returned empty values")
	}
}

func TestInMemoryRunCoverage(t *testing.T) {
	s := NewRunService()
	response := httptest.NewRecorder()
	s.execute(response, httptest.NewRequest(http.MethodPost, "/api/v1/runs/execute", strings.NewReader(`{"task_id":"task-1","idempotency_key":"key"}`)))
	if response.Code != http.StatusCreated {
		t.Fatalf("execute status = %d", response.Code)
	}
	var run RunRecord
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatal(err)
	}
	s.collection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runs?state=active&task=task-1&runner=runner&trigger=manual", nil))
	s.path(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+run.ID, nil))
	s.mu.Lock()
	s.logs[run.ID]["stdout"] = []LogChunk{{Sequence: 1, Text: "hello"}, {Sequence: 2, Text: "world"}}
	s.mu.Unlock()
	s.logsResponse(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+run.ID+"/logs?stream=stdout&after=1", nil), run.ID, false)
	s.logsResponse(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+run.ID+"/logs/download?stream=stdout", nil), run.ID, false)
	for _, action := range []string{"cancel", "retry", "reconcile"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+run.ID+"/"+action, strings.NewReader(`{"reason":"operator"}`))
		s.action(httptest.NewRecorder(), request, run.ID, action)
	}
	s.mu.Lock()
	s.runs["failed"] = RunRecord{ID: "failed", State: "FAILED", Attempt: 1}
	s.runs["unknown"] = RunRecord{ID: "unknown", State: "UNKNOWN", Attempt: 1}
	s.mu.Unlock()
	s.action(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/runs/failed/retry", strings.NewReader(`{"reason":"retry"}`)), "failed", "retry")
	s.action(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/runs/unknown/reconcile", strings.NewReader(`{"reason":"review"}`)), "unknown", "reconcile")
	if s.hasDurableRepository() {
		t.Fatal("memory run service reported durable storage")
	}
}

func TestGlobalVariableAndRoleMemoryCoverage(t *testing.T) {
	variables := NewGlobalVariableService()
	create := httptest.NewRecorder()
	variables.collection(create, httptest.NewRequest(http.MethodPost, "/api/v1/global-variables", strings.NewReader(`{"name":" BUILD_MODE ","value":"test"}`)))
	if create.Code != http.StatusCreated {
		t.Fatalf("variable create status = %d", create.Code)
	}
	var item struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &item); err != nil || item.ID == "" {
		t.Fatalf("variable = %s, err = %v", create.Body.String(), err)
	}
	variables.collection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/global-variables", nil))
	variables.path(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/global-variables/"+item.ID, nil))
	updated := httptest.NewRecorder()
	variables.path(updated, httptest.NewRequest(http.MethodPut, "/api/v1/global-variables/"+item.ID, strings.NewReader(`{"name":"BUILD_MODE","value":"prod"}`)))
	if updated.Code != http.StatusOK {
		t.Fatalf("variable update status = %d", updated.Code)
	}
	duplicate := httptest.NewRecorder()
	variables.collection(duplicate, httptest.NewRequest(http.MethodPost, "/api/v1/global-variables", strings.NewReader(`{"name":"BUILD_MODE","value":"again"}`)))
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate variable status = %d", duplicate.Code)
	}
	removed := httptest.NewRecorder()
	variables.path(removed, httptest.NewRequest(http.MethodDelete, "/api/v1/global-variables/"+item.ID, nil))
	if removed.Code != http.StatusNoContent {
		t.Fatalf("variable delete status = %d", removed.Code)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/global-variables", strings.NewReader(`{"name":"bad-name"}`)),
		httptest.NewRequest(http.MethodPut, "/api/v1/global-variables/missing", strings.NewReader(`{"name":"BAD","value":"x"}`)),
		httptest.NewRequest(http.MethodGet, "/api/v1/global-variables/a/b", nil),
	} {
		response := httptest.NewRecorder()
		if request.Method == http.MethodPost {
			variables.collection(response, request)
		} else {
			variables.path(response, request)
		}
	}

	roles := NewRoleAdminService()
	if err := roles.Seed("user", []string{"tasks.read"}); err != nil {
		t.Fatal(err)
	}
	if err := roles.Create(" Operators ", []string{"tasks.read", "tasks.read", "runs.read"}); err != nil {
		t.Fatal(err)
	}
	if err := roles.Rename("operators", "Release Operators"); err != nil {
		t.Fatal(err)
	}
	if err := roles.ReplacePermissions("release operators", []string{"runs.read"}); err != nil {
		t.Fatal(err)
	}
	if len(roles.List()) != 2 || roles.Effective("missing")["never"] {
		t.Fatal("role memory operations returned unexpected result")
	}
	if err := roles.ReplacePermissions("user", []string{"run.read"}); err == nil {
		t.Fatal("system role permissions changed")
	}
	if err := roles.Delete("user"); err == nil {
		t.Fatal("system role deleted")
	}
	if err := roles.Assign("user-1", "missing"); err == nil {
		t.Fatal("missing role assigned")
	}
	if err := roles.Unassign("user-1", "missing"); err == nil {
		t.Fatal("missing role unassigned")
	}
}

func TestInfrastructureHelperAndLeaseCoverage(t *testing.T) {
	s := NewInfrastructureService()
	if s.hasDurableRepositories() {
		t.Fatal("new infrastructure service has durable repositories")
	}
	s.SetRunnerBinaryDirectory(" ")
	s.SetRunnerControlPlaneURL(" https://control.example/ ")
	s.SetRunnerArtifactConfig(" tls://nats.example ", 2048)
	s.SetRunnerEndpointPolicy(false)
	s.SetControlPlanePublicKey(" key ")
	s.SetRunnerCapacityPublisher(nil, protocol.SigningKey{})
	if err := validateRunnerEndpoints("http://control.example", "nats://nats.example", "", "", false); err == nil {
		t.Fatal("insecure endpoints accepted")
	}
	if err := validateRunnerEndpoints("https://control.example", "tls://nats.example", "https://other.example", "", false); err == nil {
		t.Fatal("unapproved control endpoint accepted")
	}
	if err := validateRunnerEndpoints("https://control.example", "tls://nats.example", "https://control.example", "tls://nats.example", false); err != nil {
		t.Fatal(err)
	}
	if remoteAddress(nil) != "unknown" || requestBaseURL(httptest.NewRequest(http.MethodGet, "http://example", nil)) != "http://example" {
		t.Fatal("request helpers returned unexpected values")
	}
	if got := filterRunners([]RunnerRecord{{ID: "runner-1", Name: "Build", ObservedState: "ONLINE", DesiredState: "ENABLED", Pool: "default"}}, "online", "build", "enabled"); len(got) != 1 {
		t.Fatal("runner filter did not match")
	}
	if got := filterResources([]ResourceRecord{{ID: "resource-1", Name: "DB", Kind: "non_blocking"}}, "db", "non_blocking"); len(got) != 1 {
		t.Fatal("resource filter did not match")
	}

	resource := ResourceRecord{ID: "resource-coverage", Name: "Database", Kind: "exclusive", Enabled: true}
	s.mu.Lock()
	s.resources[resource.ID] = resource
	s.runners["runner-coverage"] = RunnerRecord{ID: "runner-coverage", PoolID: "default", Pool: "default", ObservedState: "REVOKED", DesiredState: "DISABLED"}
	s.enrollments["token-coverage"] = &enrollment{Token: "token-coverage", RunnerID: "runner-coverage", Expires: time.Now().Add(time.Minute)}
	s.mu.Unlock()
	if _, err := s.AcquireLease(resource.ID, "", time.Second); err != errInvalidLease {
		t.Fatalf("empty holder error = %v", err)
	}
	if _, err := s.AcquireLease("missing", "runner", time.Second); err != errResourceNotFound {
		t.Fatalf("missing resource error = %v", err)
	}
	lease, err := s.AcquireLease(resource.ID, "runner", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcquireLease(resource.ID, "other", time.Second); err != errLeaseConflict {
		t.Fatalf("lease conflict error = %v", err)
	}
	if err := s.ReleaseLease(resource.ID, "other", lease.FencingToken); err != errLeaseOwner {
		t.Fatalf("lease owner error = %v", err)
	}
	if err := s.ReleaseLease(resource.ID, "runner", lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseLease("missing", "runner", 1); err != errResourceNotFound {
		t.Fatalf("missing release error = %v", err)
	}
	if _, err := s.ConsumeEnrollment("missing", time.Now()); err != errEnrollmentNotFound {
		t.Fatalf("missing enrollment error = %v", err)
	}
	if _, err := s.ConsumeEnrollment("token-coverage", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeEnrollment("token-coverage", time.Now()); err != errEnrollmentUsed {
		t.Fatalf("used enrollment error = %v", err)
	}
	s.mu.Lock()
	s.enrollments["expired"] = &enrollment{Token: "expired", RunnerID: "runner-coverage", Expires: time.Now().Add(-time.Minute)}
	s.enrollments["missing-runner"] = &enrollment{Token: "missing-runner", RunnerID: "missing", Expires: time.Now().Add(time.Minute)}
	s.mu.Unlock()
	if _, err := s.ConsumeEnrollment("expired", time.Now()); err != errEnrollmentExpired {
		t.Fatalf("expired enrollment error = %v", err)
	}
	if _, err := s.ConsumeEnrollment("missing-runner", time.Now()); err != errEnrollmentNotFound {
		t.Fatalf("missing runner error = %v", err)
	}
	s.mu.Lock()
	s.enrollments["key-a"] = &enrollment{Token: "key-a", RunnerID: "runner-a", Expires: time.Now().Add(time.Minute)}
	s.enrollments["key-b"] = &enrollment{Token: "key-b", RunnerID: "runner-b", Expires: time.Now().Add(time.Minute)}
	s.runners["runner-a"] = RunnerRecord{ID: "runner-a"}
	s.runners["runner-b"] = RunnerRecord{ID: "runner-b"}
	s.runnerKeys["runner-a"] = runnerKey{ID: "shared-key"}
	s.mu.Unlock()
	if _, err := s.consumeEnrollmentWithKey("key-b", time.Now(), "shared-key", make([]byte, ed25519.PublicKeySize)); err == nil {
		t.Fatal("duplicate runner key accepted")
	}
	if _, err := s.consumeEnrollmentWithKey("key-a", time.Now(), "new-key", make([]byte, ed25519.PublicKeySize)); err != nil {
		t.Fatal(err)
	}
	s.MarkStale(time.Now(), time.Second)
	if resourceLeaseActive(ResourceRecord{Holder: "runner", ExpiresAt: "invalid"}, time.Now()) != true || resourceLeaseActive(ResourceRecord{}, time.Now()) {
		t.Fatal("lease activity detection failed")
	}
}

func TestOperationsValidationAndPaginationCoverage(t *testing.T) {
	o := NewOperationsService()
	if o.hasDurableRepositories() {
		t.Fatal("new operations service has durable repositories")
	}
	definition := taskDefinition("task-1", taskInput{Name: "Task", RunnerPool: "pool", Command: []string{"echo"}, Resources: []string{"resource"}})
	if definition.ID != "task-1" || validateTaskSecrets(taskInput{SecretReferences: map[string]any{"TOKEN": "secret-1"}}) != nil {
		t.Fatal("task definition validation failed")
	}
	for _, input := range []taskInput{
		{SecretReferences: map[string]any{" token ": "secret-1"}},
		{SecretReferences: map[string]any{"TOKEN": "secret..1"}},
		{SecretReferences: map[string]any{"TOKEN": 1}},
		{SecretReferences: map[string]any{"TOKEN": "secret-1"}, Environment: map[string]any{"TOKEN": "set"}},
	} {
		if validateTaskSecrets(input) == nil {
			t.Fatal("invalid task secret accepted")
		}
	}
	if err := o.validateTaskSecretIDs(context.Background(), taskInput{SecretReferences: map[string]any{"TOKEN": "secret-1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeScheduleInput(scheduleInput{DeadlineSeconds: intPtr(10)}); err == nil {
		t.Fatal("short schedule deadline accepted")
	}
	if _, err := scheduleDefinition("schedule-1", scheduleInput{Name: "Schedule", TaskID: "task-1", Expression: "bad", Timezone: "UTC"}); err == nil {
		t.Fatal("invalid schedule expression accepted")
	}
	if _, err := scheduleDefinition("schedule-1", scheduleInput{Name: "Schedule", TaskID: "task-1", Expression: "* * * * *", Timezone: "UTC"}); err != nil {
		t.Fatal(err)
	}
	if conflicts, err := o.checkScheduleConflicts(context.Background(), store.ScheduleDefinition{}); err != nil || conflicts != nil {
		t.Fatalf("unconfigured conflict check = %#v, %v", conflicts, err)
	}
	if _, err := previewOccurrences("bad", "UTC", time.Now()); err == nil {
		t.Fatal("invalid preview expression accepted")
	}
	response := httptest.NewRecorder()
	o.preview(response, httptest.NewRequest(http.MethodPost, "/api/v1/schedules/preview", strings.NewReader(`{"expression":"* * * * *","timezone":"UTC"}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d", response.Code)
	}
	o.tasks["task-1"] = TaskRecord{ID: "task-1", Name: "Build", Pool: "default", Enabled: true}
	o.tasks["task-2"] = TaskRecord{ID: "task-2", Name: "Disabled", Pool: "other", Enabled: false}
	if len(filterTasks([]TaskRecord{o.tasks["task-1"], o.tasks["task-2"]}, url.Values{"search": {"build"}, "state": {"enabled"}})) != 1 {
		t.Fatal("task filter did not match")
	}
	if len(filterSchedules([]ScheduleRecord{{ID: "due", TaskID: "task-1", Enabled: true, NextFireAt: time.Now().Add(-time.Minute).Format(time.RFC3339)}}, url.Values{"due": {"true"}, "enabled": {"true"}})) != 1 {
		t.Fatal("schedule filter did not match")
	}
	if collectionPage(httptest.NewRequest(http.MethodGet, "/?all=true", nil)); pageStart(2, 10, 5) != 5 || pageOffset(2, 10) != 10 {
		t.Fatal("pagination helpers returned unexpected result")
	}
}

func intPtr(value int) *int { return &value }

func TestOperationsInvalidRoutesCoverage(t *testing.T) {
	o := NewOperationsService()
	o.tasks["task-deleted"] = TaskRecord{ID: "task-deleted", IsDeleted: true}
	o.schedules["schedule-1"] = ScheduleRecord{ID: "schedule-1", TaskID: "task-1", Enabled: true, State: "ACTIVE"}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader("{")),
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"name":"task","command":[],"runner_pool":"pool"}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"name":"task","command":["run"],"runner_pool":"pool","secret_references":{"TOKEN":"bad..id"}}`)),
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks?archived=true", nil),
	} {
		response := httptest.NewRecorder()
		if request.Method == http.MethodGet {
			o.taskCollection(response, request)
		} else {
			o.taskCollection(response, request)
		}
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/wrong", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/tasks/missing/versions", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks/missing/versions", strings.NewReader(`{"command":[]}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/tasks/missing/versions", strings.NewReader(`{"command":["run"],"secret_references":{"TOKEN":"bad..id"}}`)),
		httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/missing", nil),
	} {
		o.taskPath(httptest.NewRecorder(), request)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader("{")),
		httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"name":"bad","task_id":"task","expression":"* * * * *","timezone":"UTC","start_deadline_seconds":10}`)),
		httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil),
	} {
		o.scheduleCollection(httptest.NewRecorder(), request)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/schedules/missing", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/schedules/missing/enable", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/schedules/missing", strings.NewReader(`{"name":"bad","task_id":"task","expression":"bad","timezone":"UTC"}`)),
		httptest.NewRequest(http.MethodPut, "/api/v1/schedules/schedule-1", nil),
		httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/missing", nil),
	} {
		o.schedulePath(httptest.NewRecorder(), request)
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/schedules/preview", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/schedules/preview", strings.NewReader(`{"timezone":"UTC"}`)),
	} {
		o.preview(httptest.NewRecorder(), request)
	}
}

func TestDurableServiceRoutesCoverage(t *testing.T) {
	db, err := store.OpenSQLite(t.TempDir() + "/api-coverage.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.ApplySQLiteMigrations(context.Background(), db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	runners := store.NewRunnerRepository(db)
	if err := runners.EnsurePool(ctx, "default", "default"); err != nil {
		t.Fatal(err)
	}
	secrets := store.NewEncryptedSecretRepository(db)
	operations := NewOperationsService()
	operations.SetTaskRepository(store.NewTaskRepository(db))
	operations.SetScheduleRepository(store.NewScheduleRepository(db))
	operations.SetResourceRepository(store.NewResourceRepository(db))
	operations.SetSecretRepository(secrets)
	if !operations.hasDurableRepositories() {
		t.Fatal("operations repositories were not configured")
	}
	createTask := httptest.NewRecorder()
	operations.taskCollection(createTask, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"name":"Durable","command":["echo","ok"],"runner_pool":"default"}`)))
	if createTask.Code != http.StatusCreated {
		t.Fatalf("durable task status = %d body=%s", createTask.Code, createTask.Body.String())
	}
	var task TaskRecord
	if err := json.Unmarshal(createTask.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	operations.taskCollection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/tasks?archived=false&search=durable", nil))
	operations.taskPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.ID, nil))
	operations.taskPath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/tasks/"+task.ID+"/versions", nil))
	version := httptest.NewRecorder()
	operations.taskPath(version, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.ID+"/versions", strings.NewReader(`{"command":["echo","new"]}`)))
	if version.Code != http.StatusCreated {
		t.Fatalf("durable version status = %d body=%s", version.Code, version.Body.String())
	}
	createSchedule := httptest.NewRecorder()
	operations.scheduleCollection(createSchedule, httptest.NewRequest(http.MethodPost, "/api/v1/schedules", strings.NewReader(`{"name":"Durable schedule","task_id":"`+task.ID+`","expression":"* * * * *","timezone":"UTC"}`)))
	if createSchedule.Code != http.StatusCreated {
		t.Fatalf("durable schedule status = %d body=%s", createSchedule.Code, createSchedule.Body.String())
	}
	var schedule ScheduleRecord
	if err := json.Unmarshal(createSchedule.Body.Bytes(), &schedule); err != nil {
		t.Fatal(err)
	}
	operations.scheduleCollection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/schedules?task="+task.ID, nil))
	operations.schedulePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/schedules/"+schedule.ID, nil))
	operations.schedulePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/schedules/"+schedule.ID+"/disable", nil))
	operations.schedulePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/schedules/"+schedule.ID+"/enable", nil))

	global := NewGlobalVariableService()
	global.SetRepository(store.NewGlobalVariableRepository(db))
	globalCreate := httptest.NewRecorder()
	global.collection(globalCreate, httptest.NewRequest(http.MethodPost, "/api/v1/global-variables", strings.NewReader(`{"name":"DURABLE_MODE","value":"on"}`)))
	if globalCreate.Code != http.StatusCreated {
		t.Fatalf("durable variable status = %d body=%s", globalCreate.Code, globalCreate.Body.String())
	}
	var variable struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(globalCreate.Body.Bytes(), &variable); err != nil {
		t.Fatal(err)
	}
	global.collection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/global-variables", nil))
	global.path(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/global-variables/"+variable.ID, nil))
	global.path(httptest.NewRecorder(), httptest.NewRequest(http.MethodPut, "/api/v1/global-variables/"+variable.ID, strings.NewReader(`{"name":"DURABLE_MODE","value":"off"}`)))
	global.path(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v1/global-variables/"+variable.ID, nil))

	infrastructure := NewInfrastructureService()
	infrastructure.SetRunnerRepository(runners)
	infrastructure.SetResourceRepository(store.NewResourceRepository(db))
	if !infrastructure.hasDurableRepositories() {
		t.Fatal("infrastructure repositories were not configured")
	}
	resourceCreate := httptest.NewRecorder()
	infrastructure.resourceCollection(resourceCreate, httptest.NewRequest(http.MethodPost, "/api/v1/resources", strings.NewReader(`{"name":"Durable resource","kind":"exclusive"}`)))
	if resourceCreate.Code != http.StatusCreated {
		t.Fatalf("durable resource status = %d body=%s", resourceCreate.Code, resourceCreate.Body.String())
	}
	var resource ResourceRecord
	if err := json.Unmarshal(resourceCreate.Body.Bytes(), &resource); err != nil {
		t.Fatal(err)
	}
	infrastructure.resourceCollection(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/resources?kind=exclusive", nil))
	infrastructure.resourcePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/resources/"+resource.ID+"/lease", strings.NewReader(`{"holder":"runner","ttl_seconds":30}`)))
	infrastructure.resourcePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v1/resources/"+resource.ID+"/lease", strings.NewReader(`{"holder":"runner","fencing_token":1}`)))
	infrastructure.resourcePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/resources/"+resource.ID, nil))
	infrastructure.resourcePath(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/api/v1/resources/"+resource.ID, nil))

	runs := NewRunService()
	runs.SetRepository(store.NewRunRepository(db))
	if !runs.hasDurableRepository() {
		t.Fatal("run repository was not configured")
	}
	runResponse := httptest.NewRecorder()
	runs.execute(runResponse, httptest.NewRequest(http.MethodPost, "/api/v1/runs/execute", strings.NewReader(`{"task_id":"`+task.ID+`","idempotency_key":"durable-key"}`)))
	if runResponse.Code != http.StatusCreated {
		t.Fatalf("durable run status = %d body=%s", runResponse.Code, runResponse.Body.String())
	}
}

func TestAuthServiceConfigurationAndIdentityCoverage(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth.SetUserRepository(nil)
	auth.SetRoleRepository(nil)
	auth.SetSessionRepository(nil)
	auth.SetSSORepository(nil)
	auth.SetConfigStore(nil)
	auth.SetAudit(func(string, string, string) {})
	if err := auth.AddRole("user", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("admin", "users.read"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetDefaultRoleID("missing"); err == nil {
		t.Fatal("missing default role accepted")
	}
	if err := auth.SetDefaultRoleID(systemRoleID("user")); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	auth.SetUserApprovalRequired(false)
	auth.SetLockdownScheduler(true)
	if !auth.LockdownScheduler() || !auth.PasswordLoginEnabled() || !auth.RegistrationEnabled() || auth.UserApprovalRequired() {
		t.Fatal("auth settings were not updated")
	}
	if err := auth.UpdateLockdownScheduler(false); err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateAuthSettings(true, false, "user", true); err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateAuthSettings(true, true, "missing"); err == nil {
		t.Fatal("missing role accepted in auth settings")
	}
	if err := auth.UpdateAuthSettings(true, true, "user", false); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetSystemAdminEmails([]string{"not-an-email"}); err == nil {
		t.Fatal("invalid system administrator email accepted")
	}
	if err := auth.SetSystemAdminEmails(nil); err != nil {
		t.Fatal(err)
	}
	if !hasSystemAdminAssignment([]store.RoleRecord{{ID: "admin", Name: "admin"}}, []store.RoleAssignmentRecord{{RoleID: "admin", SourceType: "system-admin"}}) || hasSystemAdminAssignment(nil, nil) {
		t.Fatal("system administrator assignment detection failed")
	}

	auth.SetDefaultRole("user")
	user, err := auth.Register("identity@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.LinkOIDC(user.ID, "Corp", "subject"); err != nil {
		t.Fatal(err)
	}
	if len(auth.Identities(user.ID)) != 1 || len(auth.identityProviderNames(user.ID)) != 1 {
		t.Fatal("OIDC identity was not listed")
	}
	identityID := auth.Identities(user.ID)[0]["id"].(string)
	if err := auth.UnlinkOIDC(user.ID, identityID); err != nil {
		t.Fatal(err)
	}
	if err := auth.UnlinkOIDC(user.ID, "bad"); err == nil {
		t.Fatal("invalid OIDC identity removed")
	}
	if err := auth.LinkOIDC("missing", "corp", "subject"); err == nil {
		t.Fatal("missing user linked to OIDC")
	}
	if _, ok, err := auth.UserProfile("missing"); err != nil || ok {
		t.Fatal("missing user returned a profile")
	}
	if profile, ok, err := auth.UserProfile(user.ID); err != nil || !ok || profile["email"] != user.Email {
		t.Fatalf("profile = %#v, ok=%v, err=%v", profile, ok, err)
	}
	if err := auth.UpdateProfile(user.ID, " Updated "); err != nil {
		t.Fatal(err)
	}
	if err := auth.ChangePassword(user.ID, "wrong", "new password"); err == nil {
		t.Fatal("invalid current password accepted")
	}
	if err := auth.ChangePassword(user.ID, "correct horse", "new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login("identity@example.com", "new password"); err != nil {
		t.Fatal(err)
	}
	if err := auth.DisableUser(user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login("identity@example.com", "new password"); err == nil {
		t.Fatal("login accepted while disabled")
	}
}

func TestOIDCConfigurationAndValidationCoverage(t *testing.T) {
	service := NewOIDCService()
	service.SetHTTPClient(nil)
	service.SetSecretRepository(nil, nil)
	service.SetRepository(nil)
	service.SetStateRepository(nil, nil)
	service.SetDefaultCallback(" https://app.example/callback ")
	for _, provider := range []OIDCProvider{
		{Issuer: "https://issuer.example", Callback: "https://app.example/callback"},
		{Key: "insecure", Issuer: "http://issuer.example", Callback: "https://app.example/callback"},
		{Key: "private", Issuer: "https://127.0.0.1", Callback: "https://app.example/callback"},
		{Key: "bad-callback", Issuer: "https://issuer.example", Callback: "http://app.example/callback"},
	} {
		if err := service.AddProvider(provider); err == nil {
			t.Fatalf("invalid provider accepted: %#v", provider)
		}
	}
	if err := service.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", ClientID: "client", Callback: "https://app.example/callback", Callbacks: []string{"https://app.example/callback", "https://app.example/alt"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := service.AddProvider(OIDCProvider{Key: "disabled", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if len(service.ConfiguredProviders()) != 2 || len(service.EnabledProviders()) != 1 || service.EnabledCount() != 1 || len(service.Providers()) != 1 {
		t.Fatal("OIDC provider projections are wrong")
	}
	if _, ok := service.Provider("missing"); ok {
		t.Fatal("missing OIDC provider found")
	}
	if _, err := service.Challenge("missing", "https://app.example/callback", time.Now()); err == nil {
		t.Fatal("missing OIDC provider challenged")
	}
	if _, err := service.Challenge("disabled", "https://app.example/callback", time.Now()); err == nil {
		t.Fatal("disabled OIDC provider challenged")
	}
	if _, err := service.Challenge("corp", "https://wrong.example/callback", time.Now()); err == nil {
		t.Fatal("unconfigured OIDC callback accepted")
	}
	if _, err := service.LinkChallenge("missing", "user", time.Now()); err == nil {
		t.Fatal("missing OIDC link provider accepted")
	}
	challenge, err := service.ChallengeWithPKCE("corp", "https://app.example/alt", time.Now())
	if err != nil || challenge.State == "" || challenge.Verifier == "" {
		t.Fatalf("challenge = %#v, err = %v", challenge, err)
	}
	if err := service.Complete("corp", challenge.State, "wrong", time.Now()); err == nil {
		t.Fatal("challenge with wrong nonce completed")
	}
	if _, _, _, _, err := service.CompleteAuthorizationCodeDetails("", "", "", time.Now()); err == nil {
		t.Fatal("empty OIDC authorization code accepted")
	}
	if _, err := service.SecretAttention(); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"", "http://issuer.example", "https://127.0.0.1"} {
		if _, err := secureURL(endpoint); err == nil {
			t.Fatalf("invalid OIDC endpoint accepted: %q", endpoint)
		}
	}
	if _, err := safeDialContext(context.Background(), "tcp", "invalid"); err == nil {
		t.Fatal("invalid OIDC dial address accepted")
	}
	if !isPrivateIP(net.ParseIP("127.0.0.1")) || isPrivateIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("private IP detection failed")
	}
}
