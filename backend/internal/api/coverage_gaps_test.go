package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type coverageRunRepository struct {
	store.RunRepository
	list      []store.RunRecord
	created   store.RunRecord
	createErr error
	listErr   error
	logChunks []store.RunLogChunkRecord
	logErr    error
}

func (r coverageRunRepository) List(context.Context) ([]store.RunRecord, error) {
	return r.list, r.listErr
}

func (r coverageRunRepository) Create(_ context.Context, definition store.RunDefinition) (store.RunRecord, error) {
	if r.createErr != nil {
		return store.RunRecord{}, r.createErr
	}
	r.created.ID, r.created.TaskID = definition.ID, definition.TaskID
	return r.created, nil
}

func (r coverageRunRepository) ListLogChunks(context.Context, string, string, int64) ([]store.RunLogChunkRecord, error) {
	return r.logChunks, r.logErr
}

func TestCoverageRunHelpersAndRepositoryBranches(t *testing.T) {
	testCoverageRunFilters(t)
	testCoverageRunStates(t)
	testCoverageRunCreation(t)
	testCoverageRunListing(t)
}

func testCoverageRunFilters(t *testing.T) {
	t.Helper()
	now := time.Date(2026, 9, 3, 10, 11, 12, 0, time.UTC)
	item := RunRecord{State: "RUNNING", TaskID: "task-1", TaskName: "Build Client", Runner: "runner-1", Trigger: "MANUAL", ScheduledFor: now.Format(time.RFC3339)}
	for _, test := range []struct {
		name  string
		query url.Values
		want  bool
	}{
		{"state", url.Values{"state": {"active"}}, true},
		{"task name", url.Values{"task": {"client"}}, true},
		{"task miss", url.Values{"task": {"missing"}}, false},
		{"runner miss", url.Values{"runner": {"runner-2"}}, false},
		{"trigger miss", url.Values{"trigger": {"scheduled"}}, false},
		{"from", url.Values{"from": {now.Add(time.Minute).Format(time.RFC3339)}}, false},
		{"to", url.Values{"to": {now.Add(-time.Minute).Format(time.RFC3339)}}, false},
		{"invalid time", url.Values{"from": {"invalid"}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := filterRuns([]RunRecord{item}, test.query)
			if (len(got) == 1) != test.want {
				t.Fatalf("filterRuns(%v) = %#v, want match=%t", test.query, got, test.want)
			}
		})
	}
	if _, err := parseFilterTime(""); err != nil {
		t.Fatal(err)
	}
	if parsed, err := parseFilterTime("2026-09-03T10:11"); err != nil || parsed.Hour() != 10 {
		t.Fatalf("minute time = %v, %v", parsed, err)
	}
	if _, err := parseFilterTime("invalid"); err == nil {
		t.Fatal("invalid filter time accepted")
	}
	if _, ok := checkedPaginationOffset(0, 1); ok {
		t.Fatal("invalid page accepted")
	}
	if _, ok := checkedPaginationOffset(1, 0); ok {
		t.Fatal("invalid limit accepted")
	}
	maxInt := int(^uint(0) >> 1)
	if _, ok := checkedPaginationOffset(maxInt, 2); ok {
		t.Fatal("overflowing pagination accepted")
	}
}

func testCoverageRunStates(t *testing.T) {
	t.Helper()
	for _, state := range []string{"WAITING", "DISPATCHED", "RUNNING", "RETRY_WAIT", "CANCELLING"} {
		if !isActiveRunState(state) || !runStateMatches(state, "ACTIVE") {
			t.Fatalf("active state %q was rejected", state)
		}
	}
	if isActiveRunState("DONE") || runStateMatches("DONE", "ACTIVE") || !runStateMatches("DONE", "") || !runStateMatches("DONE", "DONE") {
		t.Fatal("run state matching returned the wrong result")
	}
	response := httptest.NewRecorder()
	writeRunPage(response, 1, 50, 0, nil)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"pages":1`)) {
		t.Fatalf("empty run page = %d %s", response.Code, response.Body.String())
	}
}

func testCoverageRunCreation(t *testing.T) {
	t.Helper()
	for _, test := range []struct {
		name string
		err  error
		code int
	}{
		{"exhausted", store.ErrStorageExhausted, http.StatusServiceUnavailable},
		{"unavailable", store.ErrStorageUnavailable, http.StatusServiceUnavailable},
		{"conflict", errors.New("duplicate"), http.StatusConflict},
	} {
		t.Run("create-"+test.name, func(t *testing.T) {
			runs := NewRunService()
			response := httptest.NewRecorder()
			runs.executeDurable(response, httptest.NewRequest(http.MethodPost, "/", nil), coverageRunRepository{createErr: test.err}, "task-1", "")
			if response.Code != test.code {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
	runs := NewRunService()
	response := httptest.NewRecorder()
	runs.executeDurable(response, httptest.NewRequest(http.MethodPost, "/", nil), coverageRunRepository{created: store.RunRecord{State: "WAITING"}}, "task-1", "requested")
	if response.Code != http.StatusCreated {
		t.Fatalf("durable create status = %d", response.Code)
	}
}

func testCoverageRunListing(t *testing.T) {
	t.Helper()
	runs := NewRunService()
	response := httptest.NewRecorder()
	for _, test := range []struct {
		name string
		repo coverageRunRepository
		code int
	}{
		{"list error", coverageRunRepository{listErr: errors.New("down")}, http.StatusServiceUnavailable},
		{"log budget error", coverageRunRepository{logErr: store.ErrRunLogBudgetExceeded}, http.StatusRequestEntityTooLarge},
		{"log error", coverageRunRepository{logErr: errors.New("down")}, http.StatusServiceUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			if strings.HasPrefix(test.name, "list") {
				runs.listRepositoryRuns(response, httptest.NewRequest(http.MethodGet, "/", nil), test.repo, store.RunListFilter{}, 1, 50)
			} else {
				runs.repositoryLogsResponse(response, httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/logs", nil), "run-1", test.repo)
			}
			if response.Code != test.code {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
	response = httptest.NewRecorder()
	runs.repositoryLogsResponse(response, httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/logs/download?stream=stdout", nil), "run-1", coverageRunRepository{logChunks: []store.RunLogChunkRecord{{Sequence: 1, Text: "hello\n"}}})
	if response.Code != http.StatusOK || response.Body.String() != "hello\n" {
		t.Fatalf("download response = %d %q", response.Code, response.Body.String())
	}
}

func TestCoverageAuditAndOIDCHelpers(t *testing.T) {
	testCoverageAuditHelpers(t)
	provider := coverageTestProvider()
	testCoverageOIDCHelpers(t, provider)
	testCoverageOIDCService(t, provider)
}

func testCoverageAuditHelpers(t *testing.T) {
	t.Helper()
	now := time.Date(2026, 9, 3, 10, 11, 12, 0, time.UTC)
	event := auditEventFromStore(store.AuditEventRecord{ID: "audit-1", Method: http.MethodPost, Target: "/api/v1/tasks", ActorID: "user-1", CreatedAt: now, BeforeValue: "old", AfterValue: map[string]any{"name": "new"}})
	if event.Description != "Create task" || event.Before["value"] != "old" || event.After["name"] != "new" || event.CreatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("audit event = %#v", event)
	}
	if got := auditEventFromStore(store.AuditEventRecord{Method: http.MethodGet, Target: "/api/v1/users/u1", CreatedAt: now}); got.Description != "View user details" {
		t.Fatalf("target fallback description = %q", got.Description)
	}
	if got := toAuditMap([]string{"value"}); got["value"].([]string)[0] != "value" {
		t.Fatalf("scalar audit map = %#v", got)
	}
	audit := NewAuditQueryService()
	audit.liveLogAudits["old"] = now.Add(-liveLogAuditWindow)
	if err := audit.AddLiveLog("new", AuditEvent{ID: "new"}); err != nil {
		t.Fatal(err)
	}
	if err := audit.AddLiveLog("new", AuditEvent{ID: "duplicate"}); err != nil || len(audit.events) != 1 {
		t.Fatalf("live log deduplication: err=%v events=%#v", err, audit.events)
	}
}

func coverageTestProvider() OIDCProvider {
	return OIDCProvider{Key: "corp", Name: "Corporate", Issuer: "https://issuer.example", ClientID: "client", Callback: "https://app.example/callback", AuthURL: "https://issuer.example/auth", Audience: "aud", Enabled: true, AutoProvision: true}
}

func testCoverageOIDCHelpers(t *testing.T, provider OIDCProvider) {
	t.Helper()
	record := providerRecord(provider)
	if len(record.CallbackURLs) != 1 || record.CallbackURLs[0] != provider.Callback {
		t.Fatalf("provider record = %#v", record)
	}
	converted := providerFromRecord(record)
	if converted.Key != provider.Key || converted.Callback != provider.Callback || !converted.AutoProvision {
		t.Fatalf("provider conversion = %#v", converted)
	}
	if purposeOrLogin("link") != "link" || purposeOrLogin("other") != "login" || oidcSecretID("corp") != "oidc-provider:corp" {
		t.Fatal("OIDC helper conversion failed")
	}
	for _, value := range []string{"", "http://issuer.example", "https://127.0.0.1", "https://[::1]"} {
		if _, err := secureURL(value); err == nil {
			t.Fatalf("insecure/private URL accepted: %q", value)
		}
	}
	if value, err := secureURL("https://203.0.113.1/oidc"); err != nil || value != "https://203.0.113.1/oidc" {
		t.Fatalf("public URL = %q, %v", value, err)
	}
	if _, err := safeDialContext(context.Background(), "tcp", "127.0.0.1"); err == nil {
		t.Fatal("private OIDC dial accepted")
	}
	if _, err := safeDialContext(context.Background(), "tcp", "bad-address"); err == nil {
		t.Fatal("invalid OIDC dial accepted")
	}
	if safeTransport(nil) == nil || safeTransport(roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, nil })) == nil {
		t.Fatal("safe transport was not returned")
	}
}

func testCoverageOIDCService(t *testing.T, provider OIDCProvider) {
	t.Helper()
	service := NewOIDCService()
	if _, _, err := service.CompleteAuthorizationCode("state", "nonce", "", time.Now()); err == nil {
		t.Fatal("empty authorization code accepted")
	}
	service.providers[provider.Key] = provider
	if got, ok := service.provider(provider.Key); !ok || got.Key != provider.Key {
		t.Fatalf("memory provider = %#v, %t", got, ok)
	}
	response := httptest.NewRecorder()
	server := Server{OIDC: service}
	server.oidcProviders(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("OIDC providers status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.oidcProviders(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("OIDC providers method status = %d", response.Code)
	}
}

func TestCoverageInfrastructureAndAuthHelpers(t *testing.T) {
	testCoverageInfrastructureHelpers(t)
	testCoverageAuthHelpers(t)
}

func testCoverageInfrastructureHelpers(t *testing.T) {
	t.Helper()
	items := []RunnerRecord{{ID: "runner-1", Name: "Build", Pool: "Default", ObservedState: "RUNNING", DesiredState: "ACTIVE"}, {ID: "runner-2", Name: "Test", Pool: "CI", ObservedState: "STOPPED", DesiredState: "DRAIN"}}
	if len(filterRunners(items, "running", "build", "active")) != 1 || len(filterRunners(items, "", "missing")) != 0 || len(filterRunners(items, "", "", "drain")) != 1 {
		t.Fatalf("runner filters = %#v", items)
	}
	infrastructure := NewInfrastructureService()
	response := httptest.NewRecorder()
	infrastructure.poolCollection(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"  CI ","description":" build "}`)))
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"name":"CI"`) {
		t.Fatalf("pool create = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	infrastructure.poolCollection(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":""}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid pool status = %d", response.Code)
	}
	infrastructure.runners["archived"] = RunnerRecord{ID: "archived", IsArchived: true}
	response = httptest.NewRecorder()
	infrastructure.runnerCollection(response, httptest.NewRequest(http.MethodGet, "/?archived=true", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "archived") {
		t.Fatalf("archived runners = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	infrastructure.runnerPath(response, httptest.NewRequest(http.MethodGet, "/api/v1/runners/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing runner = %d", response.Code)
	}
}

func testCoverageAuthHelpers(t *testing.T) {
	t.Helper()
	auth, err := NewAuthService(strings.Repeat("x", 32), true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth.oidcIdentities["corp\x00subject"] = "user-1"
	auth.oidcIdentities["corp\x00other"] = "user-1"
	auth.oidcIdentities["broken"] = "user-1"
	if providers := auth.identityProviderNames("user-1"); len(providers) != 1 || providers[0] != "corp" {
		t.Fatalf("memory identity providers = %#v", providers)
	}
	auth.SetSSORepository(newMemorySSORepository())
	if providers := auth.identityProviderNames("user-1"); providers == nil {
		t.Fatal("repository identity providers returned nil")
	}
	if err := auth.SetDefaultRoleID("missing"); err == nil {
		t.Fatal("missing default role accepted")
	}
	if !hasSystemAdminAssignment([]store.RoleRecord{{ID: "admin", Name: "admin"}}, []store.RoleAssignmentRecord{{RoleID: "admin", SourceType: systemAdminSource}}) {
		t.Fatal("system admin assignment not detected")
	}
	if _, _, handled, err := auth.AdminSessionsPage("", 10, 0); err != nil || handled {
		t.Fatalf("memory admin sessions page = handled=%t err=%v", handled, err)
	}

	role := NewRoleAdminService()
	role.SetRepository(nil)
	if systemRoleID("Admin") != "system-admin" || systemRoleID("custom role") != "system-custom role" {
		t.Fatal("system role ID normalization failed")
	}
	if err := role.Seed("user", []string{"tasks.read"}); err != nil {
		t.Fatal(err)
	}
	if err := role.Delete("missing"); err == nil {
		t.Fatal("missing role deleted")
	}
}

func TestCoverageDocsAndSmallHandlers(t *testing.T) {
	response := httptest.NewRecorder()
	swaggerUI(response, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Glyphflow API") {
		t.Fatalf("swagger response = %d", response.Code)
	}
	response = httptest.NewRecorder()
	openAPI(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	var document map[string]any
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &document) != nil || document["openapi"] != "3.0.3" {
		t.Fatalf("openapi response = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	openAPI(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("openapi content type = %q", response.Header().Get("Content-Type"))
	}

	response = httptest.NewRecorder()
	server := Server{ExitCodes: testExitCodes{}}
	server.executionStatusCollection(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("execution status collection = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.executionStatusItem(response, httptest.NewRequest(http.MethodDelete, "/", nil), 127)
	if response.Code != http.StatusNoContent {
		t.Fatalf("execution status item = %d", response.Code)
	}
	if platform.PermissionCatalog == nil {
		t.Fatal("permission catalog unexpectedly nil")
	}
}

type coverageOIDCRepository struct {
	provider store.OIDCProviderRecord
	mappings []store.SSOGroupRoleMappingRecord
}

func (r *coverageOIDCRepository) Upsert(_ context.Context, provider store.OIDCProviderRecord) error {
	r.provider = provider
	return nil
}

func (r *coverageOIDCRepository) List(context.Context) ([]store.OIDCProviderRecord, error) {
	if r.provider.ID == "" {
		return nil, nil
	}
	return []store.OIDCProviderRecord{r.provider}, nil
}

func (r *coverageOIDCRepository) Find(context.Context, string) (store.OIDCProviderRecord, bool, error) {
	return r.provider, r.provider.ID != "", nil
}

func (r *coverageOIDCRepository) EnabledCount(context.Context) (int, error) { return 1, nil }

func (r *coverageOIDCRepository) ListGroupRoleMappings(context.Context, string) ([]store.SSOGroupRoleMappingRecord, error) {
	return r.mappings, nil
}

func (r *coverageOIDCRepository) SetGroupRoleMapping(context.Context, store.SSOGroupRoleMappingRecord) error {
	return nil
}

func (r *coverageOIDCRepository) ReplaceGroupRoleMappings(_ context.Context, _ string, mappings []store.SSOGroupRoleMappingRecord) error {
	r.mappings = mappings
	return nil
}

func TestCoverageOIDCRepositoryAndLinkBranches(t *testing.T) {
	repository := &coverageOIDCRepository{}
	service := NewOIDCService()
	service.SetRepository(repository)
	provider := OIDCProvider{Key: "corp", Name: "Corporate", Issuer: "https://issuer.example", ClientID: "client", Callback: "https://app.example/callback", AuthURL: "https://issuer.example/auth", Enabled: true}
	provider.GroupMapping = map[string]string{" admins ": " role-1 ", "": "ignored"}
	if err := service.saveProvider(provider, repository); err != nil {
		t.Fatal(err)
	}
	configured := service.ConfiguredProviders()
	if len(configured) != 1 || configured[0].GroupMapping["admins"] != "role-1" {
		t.Fatalf("configured providers = %#v", configured)
	}
	if got, ok := service.provider("corp"); !ok || got.Callback != provider.Callback {
		t.Fatalf("durable provider = %#v, %t", got, ok)
	}

	auth, err := NewAuthService(strings.Repeat("x", 32), true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("link@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	service = NewOIDCService()
	if err := service.AddProvider(provider); err != nil {
		t.Fatal(err)
	}
	server := Server{OIDC: service, AuthService: auth, Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: user.ID}, true }}
	response := httptest.NewRecorder()
	server.oidcLink(response, httptest.NewRequest(http.MethodGet, "/?provider=corp", nil))
	if response.Code != http.StatusFound {
		t.Fatalf("OIDC link status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.oidcLink(response, httptest.NewRequest(http.MethodPost, "/?provider=corp", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("OIDC link method status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.completeOIDCLink(response, provider, platform.OIDCClaims{Subject: "subject"}, user.ID)
	if response.Code != http.StatusOK {
		t.Fatalf("complete OIDC link = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.completeOIDCLink(response, provider, platform.OIDCClaims{}, "")
	if response.Code != http.StatusConflict {
		t.Fatalf("empty OIDC link = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.oidcLogin(response, httptest.NewRequest(http.MethodGet, "/?provider=missing&redirect_uri=https%3A%2F%2Fapp.example%2Fcallback", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid OIDC login = %d", response.Code)
	}
}

func TestCoverageSmallStateAndErrorBranches(t *testing.T) {
	deadLetters := NewDeadLetterService(nil, nil)
	deadLetters.SetRepository(&deadLetterRepositoryStub{})
	deadLetters.SetPublisher(&deadLetterPublisherStub{})
	if deadLetters.repository == nil || deadLetters.publisher == nil || (&deadLetterActionError{message: "reason"}).Error() != "reason" {
		t.Fatal("dead-letter setters or error failed")
	}
	for _, err := range []error{store.ErrExitCodeNotFound, store.ErrExitCodeSystem, store.ErrExitCodeExists, store.ErrExitCodeInUse, errors.New("invalid")} {
		response := httptest.NewRecorder()
		writeExitCodeError(response, "exit code operation", err)
		if response.Code == http.StatusOK {
			t.Fatalf("exit-code error was successful for %v", err)
		}
	}
	if NewGlobalVariableService().hasDurableRepository() {
		t.Fatal("empty global-variable service has a durable repository")
	}
	if NewRunService().hasDurableRepository() || NewAuditQueryService().hasDurableRepository() || (&SecretAdminService{}).hasDurableRepository() {
		t.Fatal("empty service reported a durable repository")
	}
	roles := NewRoleAdminService()
	if err := roles.Create("custom", nil); err != nil {
		t.Fatal(err)
	}
	definition, ok, err := roles.role("custom")
	if err != nil || !ok {
		t.Fatal(err)
	}
	if err := roles.Delete(definition.ID); err != nil {
		t.Fatal(err)
	}
	if err := roles.Delete(definition.ID); err == nil {
		t.Fatal("deleted role still exists")
	}
}

func TestCoverageRunnerMetricsBranches(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	infrastructure := NewInfrastructureService()
	infrastructure.runners["runner-1"] = RunnerRecord{ID: "runner-1"}
	infrastructure.runnerMetrics["runner-1"] = []store.RunnerMetricsRecord{{SampledAt: now.Add(-time.Minute), CPUPercent: 1}, {SampledAt: now, CPUPercent: 2}}
	if got := infrastructure.runnerWithCurrentMetrics(infrastructure.runners["runner-1"]); got.CurrentMetrics == nil || got.CurrentMetrics.CPUPercent != 2 {
		t.Fatalf("latest runner metrics = %#v", got.CurrentMetrics)
	}
	for _, query := range []string{"?limit=bad", "?limit=0", "?from=invalid", "?from=2026-09-04T00:00:00Z&to=2026-09-03T00:00:00Z"} {
		response := httptest.NewRecorder()
		infrastructure.runnerMetricsPath(response, httptest.NewRequest(http.MethodGet, "/"+query, nil), "runner-1")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid metrics query %q returned %d", query, response.Code)
		}
	}
	response := httptest.NewRecorder()
	infrastructure.runnerMetricsPath(response, httptest.NewRequest(http.MethodGet, "/?from="+now.Add(-time.Hour).Format(time.RFC3339)+"&to="+now.Add(time.Hour).Format(time.RFC3339), nil), "runner-1")
	if response.Code != http.StatusOK {
		t.Fatalf("runner metrics status = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	infrastructure.runnerMetricsPath(response, httptest.NewRequest(http.MethodGet, "/", nil), "missing")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing runner metrics = %d", response.Code)
	}
}

func TestCoverageOperationsHelpers(t *testing.T) {
	testCoverageOperationsTasks(t)
	testCoverageOperationFilters(t)
	testCoverageOperationPagination(t)
}

func testCoverageOperationsTasks(t *testing.T) {
	t.Helper()
	operations := NewOperationsService()
	created := operations.createTask(" Build ", []string{"echo", "ok"}, " default ", " runner-1 ", 30, []string{"resource-1"})
	if created.Name != "Build" || created.Pool != "default" || created.PinnedRunner != "runner-1" {
		t.Fatalf("created task = %#v", created)
	}
	if _, ok := operations.task("missing"); ok || operations.deleteTask("missing") {
		t.Fatal("missing task was found or deleted")
	}
	updated, ok := operations.addTaskVersion(created.ID, taskInput{Name: "Updated", Command: []string{"true"}, DurationSeconds: 60, Environment: map[string]any{"A": "B"}})
	if !ok || updated.Name != "Updated" || updated.ActiveVersion != 2 {
		t.Fatalf("task version update = %#v, %t", updated, ok)
	}
	if _, ok := operations.addTaskVersion("missing", taskInput{Command: []string{"true"}}); ok {
		t.Fatal("missing task version was created")
	}
	if _, err := operations.saveSchedule(scheduleInput{}, ""); err == nil {
		t.Fatal("incomplete schedule accepted")
	}
	if _, err := operations.saveSchedule(scheduleInput{TaskID: created.ID, Name: "Schedule", Expression: "0 * * * *", Timezone: "UTC"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := operations.saveSchedule(scheduleInput{TaskID: created.ID, Name: "Too Soon", Expression: "0 * * * *", Timezone: "UTC", DeadlineSeconds: coverageIntPtr(1)}, ""); err == nil {
		t.Fatal("short schedule deadline accepted")
	}
	deleted := operations.deleteTask(created.ID)
	_, stillThere := operations.task(created.ID)
	if !deleted || stillThere {
		t.Fatal("task deletion did not remove task")
	}
	if operations.deleteSchedule("missing") {
		t.Fatal("missing schedule deleted")
	}
}

func testCoverageOperationFilters(t *testing.T) {
	t.Helper()
	tasks := []TaskRecord{{ID: "task-1", Name: "Build", Pool: "default", Enabled: true}, {ID: "task-2", Name: "Test", Pool: "ci", Enabled: false}}
	if len(filterTasks(tasks, url.Values{"search": {"build"}, "state": {"enabled"}})) != 1 || len(filterTasks(tasks, url.Values{"state": {"disabled"}})) != 1 || len(filterTasks(tasks, url.Values{"search": {"missing"}})) != 0 {
		t.Fatalf("task filters = %#v", tasks)
	}
	schedules := []ScheduleRecord{{ID: "s1", TaskID: "task-1", Enabled: true, NextFireAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)}, {ID: "s2", TaskID: "task-2", Enabled: false, NextFireAt: "invalid"}}
	if len(filterSchedules(schedules, url.Values{"task": {"task-1"}, "due": {"true"}})) != 1 || len(filterSchedules(schedules, url.Values{"enabled": {"false"}})) != 1 || len(filterSchedules(schedules, url.Values{"enabled": {"invalid"}})) != 2 {
		t.Fatalf("schedule filters = %#v", schedules)
	}
	if enabled, valid := scheduleEnabledFilter(" true "); !enabled || !valid {
		t.Fatal("enabled schedule filter failed")
	}
	if scheduleMatches(schedules[0], "missing", false, false, false, time.Now()) || scheduleMatches(schedules[1], "", true, false, false, time.Now()) {
		t.Fatal("schedule matching accepted invalid filters")
	}
	if page, limit := collectionPage(httptest.NewRequest(http.MethodGet, "/?all=true&page=2&limit=1", nil)); page != 1 || limit != maxCollectionPageLimit {
		t.Fatalf("all page = %d/%d", page, limit)
	}
	if page, limit := collectionPage(httptest.NewRequest(http.MethodGet, "/?page=0&limit=1000", nil)); page != 1 || limit != 50 {
		t.Fatalf("clamped page = %d/%d", page, limit)
	}
	if pageStart(1, 10, 100) != 0 || pageStart(20, 10, 100) != 100 || pageStart(2, 10, 100) != 10 || pageOffset(2, 10) != 10 {
		t.Fatal("page helpers returned unexpected offsets")
	}
}

func testCoverageOperationPagination(t *testing.T) {
	t.Helper()
	response := httptest.NewRecorder()
	writePage(response, httptest.NewRequest(http.MethodGet, "/?page=2&limit=1", nil), []string{"a", "b"})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":["b"]`) {
		t.Fatalf("page response = %d %s", response.Code, response.Body.String())
	}
}

func coverageIntPtr(value int) *int { return &value }

type coverageUserPageRepository struct {
	store.UserRepository
	records []store.UserRecord
	err     error
}

func (r coverageUserPageRepository) ListPage(context.Context, string, string, []string, int, int) ([]store.UserRecord, int, error) {
	return r.records, len(r.records), r.err
}

func TestCoverageAuthPageAndTokenBranches(t *testing.T) {
	auth, err := NewAuthService(strings.Repeat("x", 32), true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth.SetUserRepository(nil)
	auth.SetRoleRepository(nil)
	if _, err := auth.issueTokens("user-1"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("page@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	auth.users = coverageUserPageRepository{UserRepository: auth.users, records: []store.UserRecord{{ID: user.ID, Username: user.Username, Email: user.Email, Status: store.StatusActive, Enabled: true}}}
	users, total, handled, err := auth.UsersPage("active", "page@example.com", nil, 10, 0)
	if err != nil || !handled || total != 1 || len(users) != 1 {
		t.Fatalf("user page = %#v total=%d handled=%t err=%v", users, total, handled, err)
	}
	if _, _, handled, err := auth.UsersPage("", "", nil, 10, 0); err != nil || !handled {
		t.Fatalf("second user page = handled=%t err=%v", handled, err)
	}
}

func TestCoverageRunMemoryActionBranches(t *testing.T) {
	for _, test := range []struct {
		action, state, want string
	}{
		{"cancel", "RUNNING", "CANCELLED"},
		{"retry", "FAILED", "RETRY_WAIT"},
		{"retry", "TIMED_OUT", "RETRY_WAIT"},
		{"reconcile", "UNKNOWN", "RETRY_WAIT"},
	} {
		t.Run(test.action+"-"+test.state, func(t *testing.T) {
			runs := NewRunService()
			runs.runs["run-1"] = RunRecord{ID: "run-1", State: test.state, Attempt: 1}
			response := httptest.NewRecorder()
			runs.action(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"reason":"operator"}`)), "run-1", test.action)
			if response.Code != http.StatusOK || runs.runs["run-1"].State != test.want {
				t.Fatalf("action status=%d state=%q body=%s", response.Code, runs.runs["run-1"].State, response.Body.String())
			}
		})
	}
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/", nil),
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"reason":""}`)),
	} {
		runs := NewRunService()
		runs.runs["run-1"] = RunRecord{ID: "run-1", State: "FAILED"}
		response := httptest.NewRecorder()
		runs.action(response, request, "run-1", "retry")
		if response.Code != http.StatusMethodNotAllowed && response.Code != http.StatusBadRequest {
			t.Fatalf("invalid run action status = %d", response.Code)
		}
	}
	runs := NewRunService()
	response := httptest.NewRecorder()
	runs.action(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"reason":"operator"}`)), "missing", "cancel")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing run action status = %d", response.Code)
	}
	runs.runs["run-1"] = RunRecord{ID: "run-1", State: "SUCCEEDED"}
	response = httptest.NewRecorder()
	runs.action(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"reason":"operator"}`)), "run-1", "unknown")
	if response.Code != http.StatusConflict {
		t.Fatalf("unknown run action status = %d", response.Code)
	}

	response = httptest.NewRecorder()
	runs.repositoryLogsResponse(response, httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/logs?stream=stdout", nil), "run-1", coverageRunRepository{logChunks: []store.RunLogChunkRecord{{Sequence: 3, Text: "line"}}})
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"sequence":3`) {
		t.Fatalf("repository log response = %d %s", response.Code, response.Body.String())
	}
}

func TestCoverageOperationsPathBranches(t *testing.T) {
	operations := NewOperationsService()
	task := operations.createTask("Task", []string{"true"}, "", "", 0, nil)
	operations.schedules["schedule-1"] = ScheduleRecord{ID: "schedule-1", TaskID: task.ID, Name: "Old", Expression: "0 * * * *", Timezone: "UTC", Enabled: true}
	response := httptest.NewRecorder()
	operations.taskDeletePath(response, httptest.NewRequest(http.MethodDelete, "/", nil), task.ID)
	if response.Code != http.StatusNoContent {
		t.Fatalf("task delete status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	operations.taskDeletePath(response, httptest.NewRequest(http.MethodDelete, "/", nil), "missing")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing task delete status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	operations.scheduleUpdatePath(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"New","task_id":"task-1","expression":"0 * * * *","timezone":"UTC"}`)), "schedule-1")
	if response.Code != http.StatusOK {
		t.Fatalf("schedule update status = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	operations.scheduleUpdatePath(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"bad","task_id":"task-1","expression":"0 * * * *","timezone":"UTC","start_deadline_seconds":1}`)), "schedule-1")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid schedule update status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	operations.scheduleDeletePath(response, httptest.NewRequest(http.MethodDelete, "/", nil), "schedule-1")
	if response.Code != http.StatusNoContent {
		t.Fatalf("schedule delete status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	operations.scheduleDeletePath(response, httptest.NewRequest(http.MethodDelete, "/", nil), "missing")
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing schedule delete status = %d", response.Code)
	}
}

func TestCoveragePasswordAndCurrentUserBranches(t *testing.T) {
	disabled := NewPasswordAuthService(false, true, nil)
	if err := disabled.Register("user@example.com", "correct horse"); err == nil || disabled.Verify("user@example.com", "correct horse") {
		t.Fatal("disabled password auth accepted a user")
	}
	passwords := NewPasswordAuthService(true, true, nil)
	if err := passwords.Register("bad", "correct horse"); err == nil {
		t.Fatal("invalid email accepted")
	}
	if err := passwords.Register("user@example.com", "correct horse"); err != nil {
		t.Fatal(err)
	}
	if err := passwords.Register("user@example.com", "correct horse"); err == nil || !passwords.Verify("USER@example.com", "correct horse") || passwords.Verify("user@example.com", "wrong") || passwords.Verify("missing@example.com", "correct horse") {
		t.Fatal("password registration or verification branches failed")
	}
	if !passwords.Enabled() || !passwords.RegistrationEnabled() {
		t.Fatal("password settings were not reported")
	}

	claims := Claims{UserID: "user-1", SessionID: "session-1"}
	server := Server{Auth: func(*http.Request) (Claims, bool) { return claims, true }, CurrentUser: &CurrentUserService{}}
	response := httptest.NewRecorder()
	server.currentUserProfile(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("default profile status = %d", response.Code)
	}
	server.CurrentUser.Profile = func(Claims) (map[string]any, error) { return nil, errors.New("profile down") }
	response = httptest.NewRecorder()
	server.currentUserProfile(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("profile error status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.currentUserPassword(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("password method status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.currentUserIdentity(response, httptest.NewRequest(http.MethodGet, "/api/v1/me/identities/x", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("identity method status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.revokeCurrentUserSession(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("session ownership status = %d", response.Code)
	}
	unauthenticated := Server{}
	response = httptest.NewRecorder()
	unauthenticated.requireAuthenticated(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing authenticator status = %d", response.Code)
	}
	server.Auth = func(*http.Request) (Claims, bool) { return Claims{}, false }
	response = httptest.NewRecorder()
	server.requireAuthenticated(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("rejected authenticator status = %d", response.Code)
	}

	response = httptest.NewRecorder()
	var docs Server
	docs.docsLogin(response, httptest.NewRequest(http.MethodGet, "/docs/login", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("docs login method status = %d", response.Code)
	}
	response = httptest.NewRecorder()
	docs.docsLogin(response, httptest.NewRequest(http.MethodPost, "/docs/login", strings.NewReader("not-json")))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("docs login invalid status = %d", response.Code)
	}
}

func TestCoverageAuthSettingsBootstrapAndRefresh(t *testing.T) {
	auth, err := NewAuthService(strings.Repeat("x", 32), true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.EnsureBootstrap("bad", "correct horse", "", ""); err == nil {
		t.Fatal("invalid bootstrap email accepted")
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("admin"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	bootstrap, err := auth.EnsureBootstrap("admin@example.com", "correct horse", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.DisplayName != defaultAdminDisplayName {
		t.Fatalf("bootstrap user = %#v", bootstrap)
	}
	if _, err := auth.EnsureBootstrap("admin@example.com", "correct horse", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateAuthSettings(true, false, "user", true); err != nil {
		t.Fatal(err)
	}
	if auth.RegistrationEnabled() || !auth.UserApprovalRequired() {
		t.Fatal("auth settings were not updated")
	}
	if err := auth.UpdateLockdownScheduler(true); err != nil || !auth.LockdownScheduler() {
		t.Fatalf("lockdown setting = %t, err=%v", auth.LockdownScheduler(), err)
	}
	if err := auth.Grant("missing", "admin"); err == nil {
		t.Fatal("missing user grant succeeded")
	}
	if err := auth.Revoke("missing", "admin"); err == nil {
		t.Fatal("missing user role operation succeeded")
	}
	tokens, err := auth.issueTokens(bootstrap.ID)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := auth.Refresh(tokens.SessionID, tokens.RefreshToken)
	if err != nil || rotated.SessionID == tokens.SessionID {
		t.Fatalf("refresh = %#v, err=%v", rotated, err)
	}
	if _, err := auth.Refresh(tokens.SessionID, tokens.RefreshToken); err == nil {
		t.Fatal("refresh token replay accepted")
	}
}

func TestCoverageInfrastructurePathBranches(t *testing.T) {
	infrastructure := NewInfrastructureService()
	infrastructure.pools["pool-1"] = RunnerPoolRecord{ID: "pool-1", Name: "Pool"}
	infrastructure.runners["runner-1"] = RunnerRecord{ID: "runner-1", PoolID: "pool-1", Pool: "Pool", ObservedState: "REVOKED"}
	response := httptest.NewRecorder()
	infrastructure.poolPath(response, httptest.NewRequest(http.MethodGet, "/api/v1/runners/pools/pool-1", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("pool get = %d", response.Code)
	}
	response = httptest.NewRecorder()
	infrastructure.poolPath(response, httptest.NewRequest(http.MethodPut, "/api/v1/runners/pools/pool-1", strings.NewReader(`{"name":"Renamed","description":"desc","enabled":true}`)))
	if response.Code != http.StatusOK {
		t.Fatalf("pool update = %d", response.Code)
	}
	response = httptest.NewRecorder()
	infrastructure.poolPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/pools/pool-1", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("active pool delete = %d", response.Code)
	}
	infrastructure.runners["runner-1"] = RunnerRecord{ID: "runner-1", PoolID: "pool-1", IsArchived: true, IsDeleted: true}
	response = httptest.NewRecorder()
	infrastructure.poolPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/runners/pools/pool-1", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("archived pool delete = %d", response.Code)
	}
	response = httptest.NewRecorder()
	infrastructure.poolPath(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/pools/pool-1", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("pool method = %d", response.Code)
	}

	infrastructure.runners["runner-1"] = RunnerRecord{ID: "runner-1", ObservedState: "REVOKED", DesiredState: "DISABLED", Capacity: 1}
	response = httptest.NewRecorder()
	infrastructure.runnerPath(response, httptest.NewRequest(http.MethodPut, "/api/v1/runners/runner-1", strings.NewReader(`{"capacity":2,"nats_endpoint":"nats://broker","control_plane_url":"http://control/"}`)))
	if response.Code != http.StatusOK || infrastructure.runners["runner-1"].Capacity != 2 {
		t.Fatalf("runner update = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	infrastructure.runnerPath(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/runner-1/enable", nil))
	if response.Code != http.StatusOK || infrastructure.runners["runner-1"].ObservedState != "OFFLINE" {
		t.Fatalf("runner enable = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	infrastructure.runnerPath(response, httptest.NewRequest(http.MethodPost, "/api/v1/runners/runner-1/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown runner action = %d", response.Code)
	}
	response = httptest.NewRecorder()
	infrastructure.runnerPath(response, httptest.NewRequest(http.MethodPut, "/api/v1/runners/runner-1", strings.NewReader(`{}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("empty runner update = %d", response.Code)
	}
	response = httptest.NewRecorder()
	infrastructure.resourceCollection(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","kind":"invalid"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid resource kind = %d", response.Code)
	}
	response = httptest.NewRecorder()
	infrastructure.resourceCollection(response, httptest.NewRequest(http.MethodPut, "/", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("resource method = %d", response.Code)
	}
	forwarded := httptest.NewRequest(http.MethodGet, "http://internal/", nil)
	forwarded.Header.Set("X-Forwarded-Host", "public.example")
	forwarded.Header.Set("X-Forwarded-Proto", "https")
	if requestBaseURL(forwarded) != "https://public.example" {
		t.Fatalf("forwarded base URL = %q", requestBaseURL(forwarded))
	}
}

type coverageRunnerRepository struct {
	store.RunnerRepository
	pools       []store.RunnerPoolRecord
	pool        store.RunnerPoolRecord
	poolFound   bool
	poolErr     error
	createErr   error
	updateErr   error
	deleteErr   error
	runners     []store.RunnerRecord
	runner      store.RunnerRecord
	runnerFound bool
	runnerErr   error
	listErr     error
	archivedErr error
	capacityErr error
	natsErr     error
	controlErr  error
	stateErr    error
	archiveErr  error
	archived    bool
}

func (r *coverageRunnerRepository) ListPools(context.Context) ([]store.RunnerPoolRecord, error) {
	return r.pools, r.listErr
}

func (r *coverageRunnerRepository) FindPool(context.Context, string) (store.RunnerPoolRecord, bool, error) {
	return r.pool, r.poolFound, r.poolErr
}

func (r *coverageRunnerRepository) CreatePool(context.Context, store.RunnerPoolRecord) error {
	return r.createErr
}

func (r *coverageRunnerRepository) UpdatePool(context.Context, store.RunnerPoolRecord) (store.RunnerPoolRecord, bool, error) {
	return r.pool, r.poolFound, r.updateErr
}

func (r *coverageRunnerRepository) DeletePool(context.Context, string) error { return r.deleteErr }

func (r *coverageRunnerRepository) List(context.Context) ([]store.RunnerRecord, error) {
	return r.runners, r.listErr
}

func (r *coverageRunnerRepository) ListArchived(context.Context) ([]store.RunnerRecord, error) {
	return r.runners, r.archivedErr
}

func (r *coverageRunnerRepository) Find(context.Context, string) (store.RunnerRecord, bool, error) {
	return r.runner, r.runnerFound, r.runnerErr
}

func (r *coverageRunnerRepository) SetDesiredState(context.Context, string, string) (store.RunnerRecord, bool, error) {
	return r.runner, r.runnerFound, r.stateErr
}

func (r *coverageRunnerRepository) UpdateCapacity(context.Context, string, int) (store.RunnerRecord, bool, error) {
	return r.runner, r.runnerFound, r.capacityErr
}

func (r *coverageRunnerRepository) UpdateNATSEndpoint(context.Context, string, string) (store.RunnerRecord, bool, error) {
	return r.runner, r.runnerFound, r.natsErr
}

func (r *coverageRunnerRepository) UpdateControlPlaneURL(context.Context, string, string) (store.RunnerRecord, bool, error) {
	return r.runner, r.runnerFound, r.controlErr
}

func (r *coverageRunnerRepository) Archive(context.Context, string) (bool, error) {
	return r.archived, r.archiveErr
}

func TestCoverageDurableRunnerRepositoryBranches(t *testing.T) {
	repository := coverageRunnerRepositoryFixture()
	testCoverageRunnerRoutes(t, repository)
	testCoverageRunnerErrors(t, repository)
}

func coverageRunnerRepositoryFixture() *coverageRunnerRepository {
	runner := store.RunnerRecord{ID: "runner-1", Name: "Build", Pool: "default", DesiredState: "ENABLED", ObservedState: "ONLINE", Capacity: 2}
	return &coverageRunnerRepository{
		pools:       []store.RunnerPoolRecord{{ID: "pool-1", Name: "Default", Enabled: true}},
		pool:        store.RunnerPoolRecord{ID: "pool-1", Name: "Default", Enabled: true},
		poolFound:   true,
		runners:     []store.RunnerRecord{runner},
		runner:      runner,
		runnerFound: true,
		archived:    true,
	}
}

func testCoverageRunnerRoutes(t *testing.T, repository *coverageRunnerRepository) {
	t.Helper()
	infrastructure := NewInfrastructureService()
	infrastructure.SetRunnerRepository(repository)
	request := coverageRunnerRequest

	response, req := coverageRunnerRequest(http.MethodGet, "/api/v1/runners/pools", "")
	infrastructure.poolCollection(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable pool list = %d", response.Code)
	}
	response, req = request(http.MethodGet, "/api/v1/runners", "")
	infrastructure.runnerCollection(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable runner list = %d", response.Code)
	}
	response, req = request(http.MethodGet, "/api/v1/runners?archived=true", "")
	infrastructure.runnerCollection(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable archived runner list = %d", response.Code)
	}

	response, req = request(http.MethodPost, "/api/v1/runners/pools", `{"name":"CI"}`)
	infrastructure.poolCollection(response, req)
	if response.Code != http.StatusCreated {
		t.Fatalf("durable pool create = %d", response.Code)
	}
	response, req = request(http.MethodGet, "/api/v1/runners/pools/pool-1", "")
	infrastructure.poolPath(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable pool get = %d", response.Code)
	}
	response, req = request(http.MethodPut, "/api/v1/runners/pools/pool-1", `{"name":"CI"}`)
	infrastructure.poolPath(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable pool update = %d", response.Code)
	}
	response, req = request(http.MethodDelete, "/api/v1/runners/pools/pool-1", "")
	infrastructure.poolPath(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("durable pool delete = %d", response.Code)
	}

	response, req = request(http.MethodGet, "/api/v1/runners/runner-1", "")
	infrastructure.runnerPath(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable runner get = %d", response.Code)
	}
	response, req = request(http.MethodPut, "/api/v1/runners/runner-1", `{"capacity":3,"nats_endpoint":"tls://broker","control_plane_url":"https://control/"}`)
	infrastructure.runnerPath(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable runner update = %d", response.Code)
	}
	response, req = request(http.MethodPut, "/api/v1/runners/runner-1", `{"nats_endpoint":"tls://broker"}`)
	infrastructure.runnerPath(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable runner find update = %d", response.Code)
	}
	response, req = request(http.MethodPost, "/api/v1/runners/runner-1/enable", "")
	infrastructure.runnerPath(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("durable runner state update = %d", response.Code)
	}
	response, req = request(http.MethodDelete, "/api/v1/runners/runner-1", "")
	infrastructure.runnerPath(response, req)
	if response.Code != http.StatusNoContent {
		t.Fatalf("durable runner archive = %d", response.Code)
	}
}

func coverageRunnerRequest(method, path, body string) (*httptest.ResponseRecorder, *http.Request) {
	return httptest.NewRecorder(), httptest.NewRequest(method, path, strings.NewReader(body))
}

func testCoverageRunnerErrors(t *testing.T, repository *coverageRunnerRepository) {
	t.Helper()
	request := coverageRunnerRequest
	for _, test := range []struct {
		name string
		call func(*coverageRunnerRepository)
		want int
	}{
		{"pool list error", func(r *coverageRunnerRepository) { r.listErr = errors.New("down") }, http.StatusServiceUnavailable},
		{"pool find error", func(r *coverageRunnerRepository) { r.poolErr = errors.New("down") }, http.StatusServiceUnavailable},
		{"pool update error", func(r *coverageRunnerRepository) { r.updateErr = errors.New("down") }, http.StatusConflict},
		{"pool delete conflict", func(r *coverageRunnerRepository) { r.deleteErr = store.ErrRunnerPoolInUse }, http.StatusConflict},
		{"runner list error", func(r *coverageRunnerRepository) { r.listErr = errors.New("down") }, http.StatusServiceUnavailable},
		{"runner archived error", func(r *coverageRunnerRepository) { r.archivedErr = errors.New("down") }, http.StatusServiceUnavailable},
		{"runner find error", func(r *coverageRunnerRepository) { r.runnerErr = errors.New("down") }, http.StatusServiceUnavailable},
		{"runner capacity error", func(r *coverageRunnerRepository) { r.capacityErr = errors.New("down") }, http.StatusConflict},
		{"runner nats error", func(r *coverageRunnerRepository) { r.natsErr = errors.New("down") }, http.StatusConflict},
		{"runner control error", func(r *coverageRunnerRepository) { r.controlErr = errors.New("down") }, http.StatusConflict},
		{"runner state error", func(r *coverageRunnerRepository) { r.stateErr = errors.New("down") }, http.StatusConflict},
		{"runner archive error", func(r *coverageRunnerRepository) { r.archiveErr = errors.New("down") }, http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			copy := *repository
			test.call(&copy)
			service := NewInfrastructureService()
			service.SetRunnerRepository(&copy)
			var response *httptest.ResponseRecorder
			var req *http.Request
			switch test.name {
			case "pool list error":
				response, req = request(http.MethodGet, "/api/v1/runners/pools", "")
				service.poolCollection(response, req)
			case "pool find error":
				response, req = request(http.MethodGet, "/api/v1/runners/pools/pool-1", "")
				service.poolPath(response, req)
			case "pool update error":
				response, req = request(http.MethodPut, "/api/v1/runners/pools/pool-1", `{"name":"CI"}`)
				service.poolPath(response, req)
			case "pool delete conflict":
				response, req = request(http.MethodDelete, "/api/v1/runners/pools/pool-1", "")
				service.poolPath(response, req)
			case "runner list error":
				response, req = request(http.MethodGet, "/api/v1/runners", "")
				service.runnerCollection(response, req)
			case "runner archived error":
				response, req = request(http.MethodGet, "/api/v1/runners?archived=true", "")
				service.runnerCollection(response, req)
			case "runner find error":
				response, req = request(http.MethodGet, "/api/v1/runners/runner-1", "")
				service.runnerPath(response, req)
			case "runner capacity error":
				response, req = request(http.MethodPut, "/api/v1/runners/runner-1", `{"capacity":3}`)
				service.runnerPath(response, req)
			case "runner nats error":
				response, req = request(http.MethodPut, "/api/v1/runners/runner-1", `{"nats_endpoint":"tls://broker"}`)
				service.runnerPath(response, req)
			case "runner control error":
				response, req = request(http.MethodPut, "/api/v1/runners/runner-1", `{"control_plane_url":"https://control"}`)
				service.runnerPath(response, req)
			case "runner state error":
				response, req = request(http.MethodPost, "/api/v1/runners/runner-1/enable", "")
				service.runnerPath(response, req)
			case "runner archive error":
				response, req = request(http.MethodDelete, "/api/v1/runners/runner-1", "")
				service.runnerPath(response, req)
			}
			if response.Code != test.want {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

type coverageDeadLetterRepository struct {
	store.DeadLetterRepository
	item               store.DeadLetterSummary
	findFound          bool
	listErr            error
	findErr            error
	beginErr           error
	beginClaimed       bool
	retry              store.DeadLetterRetry
	reconcileChanged   bool
	reconcileErr       error
	markPublishedErr   error
	markRetryFailedErr error
}

func (r *coverageDeadLetterRepository) List(context.Context, store.DeadLetterFilter) ([]store.DeadLetterSummary, int, error) {
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	return []store.DeadLetterSummary{r.item}, 1, nil
}

func (r *coverageDeadLetterRepository) Find(context.Context, string) (store.DeadLetterSummary, bool, error) {
	return r.item, r.findFound, r.findErr
}

func (r *coverageDeadLetterRepository) BeginRetry(context.Context, string) (store.DeadLetterRetry, bool, error) {
	return r.retry, r.beginClaimed, r.beginErr
}

func (r *coverageDeadLetterRepository) Reconcile(context.Context, string, string) (bool, error) {
	return r.reconcileChanged, r.reconcileErr
}

func (r *coverageDeadLetterRepository) MarkRetryPublished(context.Context, string) error {
	return r.markPublishedErr
}

func (r *coverageDeadLetterRepository) MarkRetryFailed(context.Context, string, string, time.Time) error {
	return r.markRetryFailedErr
}

type coverageDeadLetterPublisher struct{ err error }

func (p coverageDeadLetterPublisher) Publish(context.Context, queue.Message) error { return p.err }

func TestCoverageDeadLetterRepositoryErrorBranches(t *testing.T) {
	item := store.DeadLetterSummary{ID: "dead-1", Subject: "events", MessageID: "msg-1", State: "OPEN"}
	testCoverageDeadLetterErrors(t, item)
	testCoverageDeadLetterValidation(t, item)
}

func coverageDeadLetterRequest(method, path, body string) (*httptest.ResponseRecorder, *http.Request) {
	return httptest.NewRecorder(), httptest.NewRequest(method, path, strings.NewReader(body))
}

func testCoverageDeadLetterErrors(t *testing.T, item store.DeadLetterSummary) {
	t.Helper()
	request := coverageDeadLetterRequest
	for _, test := range []struct {
		name string
		call func(*DeadLetterService, *coverageDeadLetterRepository, *httptest.ResponseRecorder, *http.Request)
		want int
	}{
		{"collection error", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.collection(w, r)
		}, http.StatusServiceUnavailable},
		{"detail error", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.detail(w, r, "dead-1")
		}, http.StatusServiceUnavailable},
		{"detail missing", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.detail(w, r, "dead-1")
		}, http.StatusNotFound},
		{"retry begin error", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.retry(w, r, "dead-1")
		}, http.StatusServiceUnavailable},
		{"retry publish error", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.retry(w, r, "dead-1")
		}, http.StatusServiceUnavailable},
		{"retry publish state error", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.retry(w, r, "dead-1")
		}, http.StatusServiceUnavailable},
		{"reconcile find error", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.reconcile(w, r, "dead-1")
		}, http.StatusServiceUnavailable},
		{"reconcile missing", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.reconcile(w, r, "dead-1")
		}, http.StatusNotFound},
		{"reconcile update error", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.reconcile(w, r, "dead-1")
		}, http.StatusServiceUnavailable},
		{"reconcile conflict", func(s *DeadLetterService, _ *coverageDeadLetterRepository, w *httptest.ResponseRecorder, r *http.Request) {
			s.reconcile(w, r, "dead-1")
		}, http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &coverageDeadLetterRepository{item: item, findFound: true, beginClaimed: true, retry: store.DeadLetterRetry{ID: "dead-1", Subject: "events", MessageID: "msg-1", Payload: []byte("payload"), Attempts: 1}, reconcileChanged: true}
			publisher := queue.Publisher(coverageDeadLetterPublisher{})
			switch test.name {
			case "collection error":
				repository.listErr = errors.New("down")
			case "detail error", "reconcile find error":
				repository.findErr = errors.New("down")
			case "detail missing", "reconcile missing":
				repository.findFound = false
			case "retry begin error":
				repository.beginErr = errors.New("down")
			case "retry publish error":
				publisher = coverageDeadLetterPublisher{err: errors.New("publish down")}
			case "retry publish state error":
				repository.markPublishedErr = errors.New("state down")
			case "reconcile update error":
				repository.reconcileErr = errors.New("down")
			case "reconcile conflict":
				repository.reconcileChanged = false
			}
			service := NewDeadLetterService(repository, publisher)
			var response *httptest.ResponseRecorder
			var req *http.Request
			if test.name == "collection error" {
				response, req = request(http.MethodGet, "/?page=1&limit=10", "")
			} else if strings.HasPrefix(test.name, "detail") {
				response, req = request(http.MethodGet, "/", "")
			} else if strings.HasPrefix(test.name, "retry") {
				response, req = request(http.MethodPost, "/", `{"reason":"operator"}`)
			} else {
				response, req = request(http.MethodPost, "/", `{"reason":"operator","state":"RECONCILED"}`)
			}
			test.call(service, repository, response, req)
			if response.Code != test.want {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func testCoverageDeadLetterValidation(t *testing.T, item store.DeadLetterSummary) {
	t.Helper()
	request := coverageDeadLetterRequest
	service := NewDeadLetterService(&coverageDeadLetterRepository{item: item, findFound: true, beginClaimed: false}, &deadLetterPublisherStub{})
	for _, body := range []string{"not-json", `{"reason":""}`, `{"reason":"operator"}`} {
		response, req := request(http.MethodPost, "/", body)
		service.retry(response, req, "dead-1")
		if body == `{"reason":"operator"}` {
			if response.Code != http.StatusConflict {
				t.Fatalf("retry transition status = %d", response.Code)
			}
		} else if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid retry status = %d", response.Code)
		}
	}
	response, req := request(http.MethodPost, "/", `{"reason":"operator","state":"INVALID"}`)
	service.reconcile(response, req, "dead-1")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid reconcile state = %d", response.Code)
	}
	service = NewDeadLetterService(nil, nil)
	response, req = request(http.MethodGet, "/", "")
	service.collection(response, req)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("nil dead-letter repository = %d", response.Code)
	}
}

type coverageSecretRepository struct {
	store.EncryptedSecretRepository
	record    store.EncryptedSecretRecord
	findFound bool
	findErr   error
	statusErr error
	listErr   error
	upsertErr error
	deleteErr error
}

func (r *coverageSecretRepository) Upsert(context.Context, store.EncryptedSecretRecord) error {
	return r.upsertErr
}

func (r *coverageSecretRepository) Find(context.Context, string) (store.EncryptedSecretRecord, bool, error) {
	return r.record, r.findFound, r.findErr
}

func (r *coverageSecretRepository) SetIntegrityStatus(context.Context, string, string, time.Time) error {
	return r.statusErr
}

func (r *coverageSecretRepository) ListStatuses(context.Context) ([]store.EncryptedSecretStatusRecord, error) {
	return []store.EncryptedSecretStatusRecord{{ID: "secret-1", Name: "", IntegrityStatus: store.SecretIntegrityUnknown, Tasks: []store.SecretTaskUsageRecord{{ID: "task-1", Name: "Build"}}}}, r.listErr
}

func (r *coverageSecretRepository) Delete(context.Context, string) error { return r.deleteErr }

func TestCoverageSecretRepositoryAndHandlerBranches(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	validCiphertext, err := platform.EncryptSecret(key, "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	validRecord := store.EncryptedSecretRecord{ID: "secret-1", Name: "Secret", EncryptedValue: validCiphertext}
	testCoverageSecretRepositoryBranches(t, key, validRecord)
	testCoverageSecretCollectionBranches(t, key)
	testCoverageSecretPathBranches(t, key)
	testCoverageSecretAttentionBranches(t, key)
}

func testCoverageSecretRepositoryBranches(t *testing.T, key []byte, validRecord store.EncryptedSecretRecord) {
	for _, test := range []struct {
		name string
		repo coverageSecretRepository
		want int
	}{
		{"find error", coverageSecretRepository{findErr: errors.New("down")}, http.StatusBadRequest},
		{"find missing", coverageSecretRepository{}, http.StatusBadRequest},
		{"decrypt error", coverageSecretRepository{record: store.EncryptedSecretRecord{EncryptedValue: []byte("bad")}, findFound: true}, http.StatusBadRequest},
		{"status error", coverageSecretRepository{record: validRecord, findFound: true, statusErr: errors.New("down")}, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "find missing" {
				test.repo.findFound = false
			}
			if err := validateStoredSecret(context.Background(), &test.repo, key, "secret-1"); err == nil {
				t.Fatal("invalid stored secret accepted")
			}
		})
	}

	service := NewSecretAdminService(&memoryEncryptedSecretRepository{}, key)
	if _, err := service.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Upsert(context.Background(), "secret-1", "Secret", "value"); err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(context.Background(), "secret-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := (*SecretAdminService)(nil).List(context.Background()); err == nil {
		t.Fatal("nil secret service listed successfully")
	}
	if err := NewSecretAdminService(&memoryEncryptedSecretRepository{}, []byte("short")).Upsert(context.Background(), "id", "name", "value"); err == nil {
		t.Fatal("short-key secret service upserted successfully")
	}
	if err := service.Upsert(context.Background(), "", "name", "value"); err == nil {
		t.Fatal("empty secret ID accepted")
	}
	if err := service.Delete(context.Background(), ""); err == nil {
		t.Fatal("empty secret ID deleted")
	}
}

func testCoverageSecretCollectionBranches(t *testing.T, key []byte) {
	server := Server{Secrets: NewSecretAdminService(&coverageSecretRepository{listErr: errors.New("down")}, key)}
	response := httptest.NewRecorder()
	server.secretCollection(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("secret list error = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.secretCollection(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","secret_value":"y","extra":true}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown secret field = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.secretCollection(response, httptest.NewRequest(http.MethodPut, "/", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("secret collection method = %d", response.Code)
	}

	server.Secrets = NewSecretAdminService(&coverageSecretRepository{upsertErr: errors.New("down")}, key)
	response = httptest.NewRecorder()
	server.secretCollection(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"x","secret_value":"y"}`)))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("secret create error = %d", response.Code)
	}
}

func testCoverageSecretPathBranches(t *testing.T, key []byte) {
	server := Server{Secrets: NewSecretAdminService(&memoryEncryptedSecretRepository{}, key)}
	response := httptest.NewRecorder()
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{"in use", store.ErrEncryptedSecretInUse, http.StatusConflict},
		{"missing", store.ErrEncryptedSecretNotFound, http.StatusNotFound},
		{"storage", errors.New("down"), http.StatusServiceUnavailable},
	} {
		server.Secrets = NewSecretAdminService(&coverageSecretRepository{deleteErr: test.err}, key)
		response = httptest.NewRecorder()
		server.secretPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/secrets/secret-1", nil))
		if response.Code != test.want {
			t.Fatalf("secret delete %s = %d", test.name, response.Code)
		}
	}
	server.Secrets = NewSecretAdminService(&memoryEncryptedSecretRepository{}, key)
	response = httptest.NewRecorder()
	server.secretPath(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/secrets/secret-1", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("secret path method = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.secretPath(response, httptest.NewRequest(http.MethodPut, "/api/v1/admin/secrets/secret-1", strings.NewReader("bad")))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("secret update decode = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.secretPath(response, httptest.NewRequest(http.MethodDelete, "/api/v1/admin/secrets/secret-1/extra", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("secret malformed path = %d", response.Code)
	}
}

func testCoverageSecretAttentionBranches(t *testing.T, key []byte) {
	server := Server{Secrets: NewSecretAdminService(&coverageSecretRepository{}, key)}
	response := httptest.NewRecorder()
	server.secretAttention(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "secret-1") {
		t.Fatalf("secret attention = %d %s", response.Code, response.Body.String())
	}
	server.Secrets = NewSecretAdminService(&coverageSecretRepository{listErr: errors.New("down")}, key)
	response = httptest.NewRecorder()
	server.secretAttention(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("secret attention error = %d", response.Code)
	}
	response = httptest.NewRecorder()
	server.secretAttention(response, httptest.NewRequest(http.MethodPost, "/", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("secret attention method = %d", response.Code)
	}
	response = httptest.NewRecorder()
	(Server{}).secretAttention(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing secret attention service = %d", response.Code)
	}
}

func TestCoverageOIDCFetchAndDiscoveryBranches(t *testing.T) {
	service := NewOIDCService()
	testCoverageOIDCFetchBranches(t, service)
	testCoverageOIDCExchangeBranches(t, service)
	testCoverageOIDCDiscoveryBranches(t, service)
}

func testCoverageOIDCFetchBranches(t *testing.T, service *OIDCService) {
	if _, err := service.fetch("http://issuer.example", nil); err == nil {
		t.Fatal("insecure OIDC fetch accepted")
	}
	for _, test := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"provider error", http.StatusBadGateway, `{}`, "provider request failed"},
		{"invalid JSON", http.StatusOK, "not-json", "response is invalid"},
		{"valid JSON", http.StatusOK, `{"ok":true}`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(test.body)), Header: make(http.Header)}, nil
			})})
			body, err := service.fetch("https://issuer.example/data", nil)
			if test.want == "" {
				if err != nil || string(body) != test.body {
					t.Fatalf("fetch = %q, %v", body, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("fetch error = %v", err)
			}
		})
	}
	service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport down")
	})})
	if _, err := service.fetch("https://issuer.example/data", nil); err == nil {
		t.Fatal("transport error was swallowed")
	}
	service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxOIDCResponseBytes+1))), Header: make(http.Header)}, nil
	})})
	if _, err := service.fetch("https://issuer.example/data", nil); err == nil {
		t.Fatal("oversized OIDC response accepted")
	}
}

func testCoverageOIDCExchangeBranches(t *testing.T, service *OIDCService) {
	provider := OIDCProvider{Key: "corp", Issuer: "https://issuer.example", ClientID: "client"}
	service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			return nil, errors.New("request was not a form POST")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id_token":"token"}`)), Header: make(http.Header)}, nil
	})})
	if token, err := service.exchangeCode(provider, "https://issuer.example/token", "code", "https://app.example/callback", "verifier"); err != nil || token != "token" {
		t.Fatalf("exchange code = %q, %v", token, err)
	}
	service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"access_token":"missing"}`)), Header: make(http.Header)}, nil
	})})
	if _, err := service.exchangeCode(provider, "https://issuer.example/token", "code", "https://app.example/callback", "verifier"); err == nil {
		t.Fatal("token response without ID token accepted")
	}
}

func testCoverageOIDCDiscoveryBranches(t *testing.T, service *OIDCService) {
	provider := OIDCProvider{Key: "corp", Issuer: "https://issuer.example", ClientID: "client"}
	for _, body := range []string{
		"not-json",
		`{"issuer":"https://other.example","token_endpoint":"https://issuer.example/token","jwks_uri":"https://issuer.example/jwks"}`,
		`{"issuer":"https://issuer.example","token_endpoint":"http://issuer.example/token","jwks_uri":"https://issuer.example/jwks"}`,
		`{"issuer":"https://issuer.example","token_endpoint":"https://issuer.example/token","jwks_uri":"http://issuer.example/jwks"}`,
	} {
		service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})})
		if _, err := service.discovery(provider); err == nil {
			t.Fatalf("invalid OIDC metadata accepted: %s", body)
		}
	}
	service.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"issuer":"https://issuer.example","token_endpoint":"https://issuer.example/token","jwks_uri":"https://issuer.example/jwks"}`)), Header: make(http.Header)}, nil
	})})
	if metadata, err := service.discovery(provider); err != nil || metadata.TokenEndpoint == "" || metadata.JWKSURI == "" {
		t.Fatalf("discovery = %#v, %v", metadata, err)
	}
	if _, err := safeDialContext(context.Background(), "tcp", "localhost:1"); err == nil {
		t.Fatal("localhost OIDC dial accepted")
	}
}

type coverageSessionRepository struct {
	store.SessionRepository
	session      store.SessionRecord
	found        bool
	getErr       error
	createErr    error
	rotateErr    error
	listRecords  []store.SessionRecord
	listErr      error
	adminRecords []store.AdminSessionRecord
	adminErr     error
	active       bool
	activeErr    error
}

func (r *coverageSessionRepository) Create(context.Context, store.SessionRecord) error {
	return r.createErr
}

func (r *coverageSessionRepository) Get(context.Context, string) (store.SessionRecord, bool, error) {
	return r.session, r.found, r.getErr
}

func (r *coverageSessionRepository) Rotate(context.Context, string, string, store.SessionRecord) error {
	return r.rotateErr
}

func (r *coverageSessionRepository) List(context.Context, string) ([]store.SessionRecord, error) {
	return r.listRecords, r.listErr
}

func (r *coverageSessionRepository) ListAdminPage(context.Context, string, int, int) ([]store.AdminSessionRecord, int, error) {
	return r.adminRecords, len(r.adminRecords), r.adminErr
}

func (r *coverageSessionRepository) Active(context.Context, string, string) (bool, error) {
	return r.active, r.activeErr
}

func TestCoverageDurableAuthTokenBranches(t *testing.T) {
	repository := &coverageSessionRepository{session: store.SessionRecord{ID: "session-1", UserID: "user-1", RefreshExpiresAt: time.Now().Add(time.Hour)}, found: true}
	testCoverageDurableAuthTokenSuccess(t, repository)
	testCoverageDurableAuthTokenFailures(t, repository.session)
}

func testCoverageDurableAuthTokenSuccess(t *testing.T, repository *coverageSessionRepository) {
	auth, err := NewAuthService(strings.Repeat("x", 32), true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth.SetUserRepository(auth.users)
	auth.SetRoleRepository(auth.roles)
	auth.SetSessionRepository(repository)
	tokens, err := auth.issueTokens("user-1")
	if err != nil || tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.SessionID == "" {
		t.Fatalf("durable issue tokens = %#v, %v", tokens, err)
	}
	rotated, err := auth.Refresh("session-1", "refresh-token")
	if err != nil || rotated.SessionID == "" || rotated.SessionID == "session-1" {
		t.Fatalf("durable refresh = %#v, %v", rotated, err)
	}
}

func testCoverageDurableAuthTokenFailures(t *testing.T, session store.SessionRecord) {
	for _, test := range []struct {
		name string
		repo coverageSessionRepository
		call func(*AuthService) error
	}{
		{"issue error", coverageSessionRepository{createErr: errors.New("down")}, func(s *AuthService) error { _, err := s.issueTokens("user-1"); return err }},
		{"refresh get error", coverageSessionRepository{getErr: errors.New("down")}, func(s *AuthService) error { _, err := s.Refresh("session-1", "refresh-token"); return err }},
		{"refresh missing", coverageSessionRepository{}, func(s *AuthService) error { _, err := s.Refresh("session-1", "refresh-token"); return err }},
		{"refresh rotate error", coverageSessionRepository{session: session, found: true, rotateErr: errors.New("down")}, func(s *AuthService) error { _, err := s.Refresh("session-1", "refresh-token"); return err }},
		{"refresh replay", coverageSessionRepository{session: session, found: true, rotateErr: store.ErrSessionReplay}, func(s *AuthService) error { _, err := s.Refresh("session-1", "refresh-token"); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewAuthService(strings.Repeat("x", 32), true, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "refresh replay" {
				service.SetAudit(func(string, string, string) {})
			}
			service.SetSessionRepository(&test.repo)
			if err := test.call(service); err == nil {
				t.Fatal("repository failure was not returned")
			}
		})
	}
}

func TestCoverageSessionManagerRepositoryAndAuthenticatorBranches(t *testing.T) {
	manager, err := NewSessionManager(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Issue("user-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	if sessions, err := manager.List("user-1"); err != nil || len(sessions) != 1 {
		t.Fatalf("memory sessions = %#v, %v", sessions, err)
	}
	if sessions, err := manager.List("missing"); err != nil || len(sessions) != 0 {
		t.Fatalf("missing memory sessions = %#v, %v", sessions, err)
	}
	if _, _, handled, err := manager.AdminPage("", 10, 0); err != nil || handled {
		t.Fatalf("unpaged admin sessions = handled=%t err=%v", handled, err)
	}

	now := time.Now().UTC()
	repository := &coverageSessionRepository{
		found:        true,
		session:      store.SessionRecord{ID: "session-1", UserID: "user-1", AccessExpiresAt: now.Add(time.Hour), LastSeenAt: now, UserAgent: "test", IPAddress: "127.0.0.1"},
		listRecords:  []store.SessionRecord{{ID: "session-1", AccessExpiresAt: now.Add(time.Hour), LastSeenAt: now, UserAgent: "test", IPAddress: "127.0.0.1"}},
		adminRecords: []store.AdminSessionRecord{{ID: "session-1", UserID: "user-1", UserEmail: "user@example.com", ExpiresAt: now.Add(time.Hour), LastSeenAt: now}},
	}
	manager.SetRepository(repository)
	if !manager.Owns("user-1", "session-1") || manager.Owns("other", "session-1") {
		t.Fatal("durable session ownership was incorrect")
	}
	if sessions, err := manager.List("user-1"); err != nil || len(sessions) != 1 || sessions[0].UserAgent != "test" {
		t.Fatalf("durable sessions = %#v, %v", sessions, err)
	}
	if sessions, total, handled, err := manager.AdminPage("user@example.com", 10, 0); err != nil || !handled || total != 1 || len(sessions) != 1 {
		t.Fatalf("durable admin sessions = %#v total=%d handled=%t err=%v", sessions, total, handled, err)
	}
	if _, ok := manager.Authenticator()(httptest.NewRequest(http.MethodGet, "/", nil)); ok {
		t.Fatal("missing session credentials accepted")
	}
	manager.SetRepository(&coverageSessionRepository{found: true, session: repository.session, getErr: errors.New("down")})
	if manager.Owns("user-1", "session-1") {
		t.Fatal("session repository error reported ownership")
	}
	authenticatorRepository := &coverageSessionRepository{active: true}
	manager.SetRepository(authenticatorRepository)
	token, _, err := manager.IssueForSession("user-1", "session-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	if claims, ok := manager.Authenticator()(request); !ok || claims.UserID != "user-1" {
		t.Fatalf("valid durable token = %#v, %t", claims, ok)
	}
	authenticatorRepository.active = false
	if _, ok := manager.Authenticator()(request); ok {
		t.Fatal("inactive durable token accepted")
	}
	authenticatorRepository.active, authenticatorRepository.activeErr = true, errors.New("down")
	if _, ok := manager.Authenticator()(request); ok {
		t.Fatal("durable token repository error accepted")
	}
}

func TestCoveragePasswordRouteBranches(t *testing.T) {
	password := NewPasswordAuthService(true, true, nil)
	sessions, err := NewSessionManager(strings.Repeat("x", 32))
	if err != nil {
		t.Fatal(err)
	}
	handler := (Server{PasswordAuth: password, Sessions: sessions}).Handler()
	request := func(method, path, body string) *http.Request {
		return httptest.NewRequest(method, path, strings.NewReader(body))
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request(http.MethodPost, "/api/v1/auth/register", `{"email":"user@example.com","password":"correct horse"}`))
	if response.Code != http.StatusCreated {
		t.Fatalf("password route register = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request(http.MethodPost, "/api/v1/auth/login", `{"email":"user@example.com","password":"correct horse"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("password route login = %d %s", response.Code, response.Body.String())
	}
	for _, path := range []string{"/api/v1/auth/register", "/api/v1/auth/login"} {
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, request(http.MethodGet, path, ""))
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("password route method %s = %d", path, response.Code)
		}
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request(http.MethodPost, "/api/v1/auth/register", "not-json"))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("password route malformed register = %d", response.Code)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request(http.MethodPost, "/api/v1/auth/login", "not-json"))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("password route malformed login = %d", response.Code)
	}
	noSessions := (Server{PasswordAuth: password}).Handler()
	response = httptest.NewRecorder()
	noSessions.ServeHTTP(response, request(http.MethodPost, "/api/v1/auth/login", `{"email":"user@example.com","password":"correct horse"}`))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("password route without sessions = %d", response.Code)
	}
}

type coverageExitCodeRepository struct {
	store.ExitCodeRepository
	listErr   error
	createErr error
	updateErr error
	deleteErr error
}

func (r coverageExitCodeRepository) List(context.Context) ([]store.ExitCodeRecord, error) {
	return nil, r.listErr
}

func (r coverageExitCodeRepository) Create(context.Context, int, string) (store.ExitCodeRecord, error) {
	return store.ExitCodeRecord{}, r.createErr
}

func (r coverageExitCodeRepository) Update(context.Context, int, int, string) (store.ExitCodeRecord, error) {
	return store.ExitCodeRecord{}, r.updateErr
}

func (r coverageExitCodeRepository) Delete(context.Context, int) error { return r.deleteErr }

func TestCoverageExecutionStatusErrorBranches(t *testing.T) {
	server := Server{}
	response := httptest.NewRecorder()
	server.executionStatus(response, httptest.NewRequest(http.MethodGet, executionStatusPath, nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing exit-code service = %d", response.Code)
	}
	for _, test := range []struct {
		name string
		repo coverageExitCodeRepository
		call func(Server, http.ResponseWriter, *http.Request)
		want int
	}{
		{"list", coverageExitCodeRepository{listErr: errors.New("down")}, func(s Server, w http.ResponseWriter, r *http.Request) { s.executionStatusCollection(w, r) }, http.StatusServiceUnavailable},
		{"create", coverageExitCodeRepository{createErr: store.ErrExitCodeExists}, func(s Server, w http.ResponseWriter, r *http.Request) { s.executionStatusCollection(w, r) }, http.StatusConflict},
		{"update", coverageExitCodeRepository{updateErr: store.ErrExitCodeNotFound}, func(s Server, w http.ResponseWriter, r *http.Request) { s.executionStatusItem(w, r, 1) }, http.StatusNotFound},
		{"delete", coverageExitCodeRepository{deleteErr: store.ErrExitCodeInUse}, func(s Server, w http.ResponseWriter, r *http.Request) { s.executionStatusItem(w, r, 1) }, http.StatusConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := Server{ExitCodes: test.repo}
			var request *http.Request
			if test.name == "list" {
				request = httptest.NewRequest(http.MethodGet, executionStatusPath, nil)
			} else if test.name == "create" {
				request = httptest.NewRequest(http.MethodPost, executionStatusPath, strings.NewReader(`{"code":1,"meaning":"failure"}`))
			} else if test.name == "update" {
				request = httptest.NewRequest(http.MethodPut, executionStatusPath+"/1", strings.NewReader(`{"meaning":"failure"}`))
			} else {
				request = httptest.NewRequest(http.MethodDelete, executionStatusPath+"/1", nil)
			}
			response := httptest.NewRecorder()
			test.call(server, response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
	server = Server{ExitCodes: testExitCodes{}}
	for _, test := range []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodPost, executionStatusPath, "bad", http.StatusBadRequest},
		{http.MethodPost, executionStatusPath, `{"meaning":"missing code"}`, http.StatusBadRequest},
		{http.MethodPut, executionStatusPath + "/1", "bad", http.StatusBadRequest},
		{http.MethodGet, executionStatusPath + "/not-a-code", "", http.StatusNotFound},
		{http.MethodGet, executionStatusPath + "/1/extra", "", http.StatusNotFound},
		{http.MethodGet, executionStatusPath, "", http.StatusOK},
	} {
		response := httptest.NewRecorder()
		server.executionStatus(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != test.want {
			t.Fatalf("%s %s = %d, want %d", test.method, test.path, response.Code, test.want)
		}
	}
}

func TestCoverageRuntimeConfigWithConfiguredAuth(t *testing.T) {
	auth, err := NewAuthService(strings.Repeat("x", 32), true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth.SetUserApprovalRequired(true)
	auth.SetLockdownScheduler(true)
	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := Server{AuthService: auth, PasswordAuth: NewPasswordAuthService(false, false, nil), OIDC: oidc}
	response := httptest.NewRecorder()
	server.runtimeConfig(response, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"oidc":true`) {
		t.Fatalf("configured runtime config = %d %s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	server.runtimeConfig(response, httptest.NewRequest(http.MethodPost, "/api/v1/config", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("runtime config method = %d", response.Code)
	}
}

func TestCoverageRunCollectionValidationBranches(t *testing.T) {
	runs := NewRunService()
	response := httptest.NewRecorder()
	runs.collection(response, httptest.NewRequest(http.MethodPost, "/api/v1/runs", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("run collection method = %d", response.Code)
	}
	response = httptest.NewRecorder()
	runs.collection(response, httptest.NewRequest(http.MethodGet, "/api/v1/runs?page=9223372036854775807&limit=2", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("run collection invalid pagination = %d", response.Code)
	}
}
