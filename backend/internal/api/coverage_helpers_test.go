package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

func TestAuditDescriptionBranches(t *testing.T) {
	cases := []struct {
		method, path, want string
	}{
		{http.MethodGet, "/api/v1/tasks", "List tasks"},
		{http.MethodPost, "/api/v1/admin/auth/settings", "Update authentication settings"},
		{http.MethodGet, "/api/v1/admin/auth/users/u1", "Disable user"},
		{http.MethodDelete, "/api/v1/admin/roles/r1", "Delete role"},
		{http.MethodPatch, "/api/v1/admin/roles/r1", "Update role"},
		{http.MethodGet, "/api/v1/users/u1", "View user details"},
		{http.MethodGet, "/api/v1/me/profile", "Manage own account"},
		{http.MethodDelete, "/api/v1/schedules/s1", "Delete schedule"},
		{http.MethodGet, "/api/v1/schedules/s1", "View schedule"},
		{http.MethodPatch, "/api/v1/schedules/s1", "Update schedule"},
		{http.MethodGet, "/api/v1/tasks/t1/versions", "Publish task version"},
		{http.MethodPost, "/api/v1/tasks/t1/cancel", "Cancel task run"},
		{http.MethodPost, "/api/v1/tasks/t1/retry", "Retry task run"},
		{http.MethodDelete, "/api/v1/tasks/t1", "Delete task"},
		{http.MethodGet, "/api/v1/tasks/t1", "View task"},
		{http.MethodPatch, "/api/v1/tasks/t1", "Update task"},
		{http.MethodPost, "/api/v1/resources/r1/lease", "Acquire resource lease"},
		{http.MethodDelete, "/api/v1/resources/r1/lease", "Release resource lease"},
		{http.MethodGet, "/api/v1/resources/r1", "View resource"},
		{http.MethodDelete, "/api/v1/resources/r1", "Delete resource"},
		{http.MethodPatch, "/api/v1/resources/r1", "Update resource"},
		{http.MethodPost, "/api/v1/runners/r1/enrollments", "Create runner enrollment"},
		{http.MethodDelete, "/api/v1/runners/r1", "Delete runner"},
		{http.MethodPost, "/api/v1/runners/r1/enable", "Enable runner"},
		{http.MethodGet, "/api/v1/runners/r1", "View runner"},
		{http.MethodPatch, "/api/v1/runners/r1", "Update runner"},
		{http.MethodGet, "/api/v1/runs/r1/logs/download", "Download run logs"},
		{http.MethodGet, "/api/v1/runs/r1/logs", "Stream run logs"},
		{http.MethodGet, "/api/v1/runs/r1/events", "List run events"},
		{http.MethodPost, "/api/v1/runs/r1/cancel", "Cancel run"},
		{http.MethodPost, "/api/v1/runs/r1/retry", "Retry run"},
		{http.MethodPost, "/api/v1/runs/r1/reconcile", "Reconcile run"},
		{http.MethodGet, "/api/v1/runs/r1", "View run"},
		{http.MethodPatch, "/api/v1/runs/r1", "Manage run"},
		{http.MethodPatch, "/unknown", "PATCH /unknown"},
	}
	for _, test := range cases {
		t.Run(test.method+test.path, func(t *testing.T) {
			if got := auditDescription(test.method, test.path); got != test.want {
				t.Fatalf("auditDescription() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAuditDataAndRedactionHelpers(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	event := AuditEvent{Actor: "user-1", Action: http.MethodPost, Target: "/api/v1/tasks/t1", Result: "success", CorrelationID: "corr-1", CreatedAt: created.Format(time.RFC3339Nano)}
	filters := map[string]string{"actor": "USER", "action": "post", "target": "tasks", "result": "SUCCESS", "correlationId": "CORR"}
	testAuditMatchingAndPagination(t, event, filters, created)
	testAuditRedactionHelpers(t)
}

func testAuditMatchingAndPagination(t *testing.T, event AuditEvent, filters map[string]string, created time.Time) {
	if !auditMatches(event, filters, created, created.Add(-time.Second), created.Add(time.Second)) {
		t.Fatal("matching audit event was rejected")
	}
	if auditMatches(event, map[string]string{"target": "missing"}, created, time.Time{}, time.Time{}) {
		t.Fatal("non-matching audit event was accepted")
	}
	if !auditEventExcluded(event, auditQueryOptions{excludeMethod: http.MethodPost, filters: filters}) {
		t.Fatal("excluded audit method was not excluded")
	}
	if !auditEventExcluded(AuditEvent{Target: "/api/v1/runs/r1/logs", CreatedAt: event.CreatedAt}, auditQueryOptions{excludeRunLogs: true}) {
		t.Fatal("run-log audit event was not excluded")
	}
	if !auditEventExcluded(AuditEvent{CreatedAt: "invalid"}, auditQueryOptions{}) {
		t.Fatal("invalid audit timestamp was not excluded")
	}
	if !isRunLogAudit("/api/v1/tasks/t1", "/api/v1/runs/r1/logs/") || isRunLogAudit("/api/v1/tasks/t1") {
		t.Fatal("run-log detection returned the wrong result")
	}

	for _, test := range []struct {
		all                            bool
		wantPage, wantLimit, wantPages int
		page, limit, itemCount, total  int
	}{
		{false, 1, 50, 1, 0, 0, 0, 0},
		{false, 3, 20, 3, 3, 20, 0, 41},
		{true, 1, 1000, 2, 0, 2000, 2000, 2000},
	} {
		got := normalizeAuditPagination(test.all, test.page, test.limit, test.itemCount, test.total)
		if got.page != test.wantPage || got.limit != test.wantLimit || got.pages != test.wantPages {
			t.Fatalf("normalizeAuditPagination() = %+v", got)
		}
	}
}

func testAuditRedactionHelpers(t *testing.T) {
	redacted := redactAuditMap(map[string]any{
		"password":             "hidden",
		"passwordLoginEnabled": true,
		"nested":               map[string]any{"token": "hidden"},
		"items":                []any{"authorization: hidden", "visible"},
	})
	if redacted["password"] != "[REDACTED]" || redacted["passwordLoginEnabled"] != true ||
		redacted["nested"].(map[string]any)["token"] != "[REDACTED]" ||
		!reflect.DeepEqual(redacted["items"], []any{"[REDACTED]", "visible"}) {
		t.Fatalf("redactAuditMap() = %#v", redacted)
	}
	if redactAuditMap(nil) != nil || redactAuditValue(42) != 42 || redactSensitiveText("safe") != "safe" || redactSensitiveText("bearer secret") != "[REDACTED]" {
		t.Fatal("redaction helpers returned unexpected values")
	}
	if _, err := parseAuditTime("not-a-time"); err == nil {
		t.Fatal("invalid audit time was accepted")
	}
	if value, err := parseAuditTime(""); err != nil || !value.IsZero() {
		t.Fatalf("empty audit time = %v, %v", value, err)
	}
}

func TestAPIResponseAndRequestHelpers(t *testing.T) {
	testAPIPermissionAndSnapshotHelpers(t)
	testAPIRequestAuditHelpers(t)
	testAPIResponseHelpers(t)
}

func testAPIPermissionAndSnapshotHelpers(t *testing.T) {
	if !hasPermission(map[string]bool{"tasks.read": true}, "task.read") ||
		!hasPermission(map[string]bool{"runs.retry": true}, "runs.read|run.retry") ||
		hasPermission(nil, "users.manage") {
		t.Fatal("permission aliases returned the wrong result")
	}
	if got := auditSnapshot(map[string]any{"name": "task"}); got["name"] != "task" {
		t.Fatalf("auditSnapshot() = %#v", got)
	}
	if got := auditSnapshot("value"); got["value"] != "value" {
		t.Fatalf("scalar auditSnapshot() = %#v", got)
	}
	if got := auditSnapshot(func() {}); got["value"] == nil || auditFind(nil, false) != nil {
		t.Fatal("audit snapshot fallback was not used")
	}
}

func testAPIRequestAuditHelpers(t *testing.T) {
	details := &requestAuditDetails{Input: map[string]any{}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(`{"name":"task"}`))
	request = request.WithContext(context.WithValue(request.Context(), requestAuditContextKey{}, details))
	recordRequestAuditField(request, "taskId", "task-1")
	recordRequestError(request, errors.New("token should be hidden"))
	if details.Input["taskId"] != "task-1" || details.Error != "[REDACTED]" || details.Traceback == "" {
		t.Fatalf("request audit details = %#v", details)
	}
	input := captureAuditInput(request)
	if input["endpoint"] != "/api/v1/tasks" || input["method"] != http.MethodPost || input["body"].(map[string]any)["name"] != "task" {
		t.Fatalf("captured audit input = %#v", input)
	}
	if !reflect.DeepEqual(auditInput(request), details.Input) {
		t.Fatal("context audit input was not reused")
	}
	if body := captureAuditInput(httptest.NewRequest(http.MethodGet, "/healthz", strings.NewReader("not-json"))); body["body"] != "not-json" {
		t.Fatalf("invalid audit body = %#v", body)
	}
}

func testAPIResponseHelpers(t *testing.T) {
	for _, test := range []struct {
		body, want string
	}{
		{`{"error":"bad"}`, "bad"},
		{`{"message":"bad"}`, "bad"},
		{"plain", "plain"},
		{"", "Not Found"},
	} {
		if got := auditResponseError([]byte(test.body), http.StatusNotFound); got != test.want {
			t.Fatalf("auditResponseError(%q) = %q, want %q", test.body, got, test.want)
		}
	}
	if auditResponseBody(nil) != nil || auditResponseBody([]byte(`{"ok":true}`)).(map[string]any)["ok"] != true || auditResponseBody([]byte("plain")) != "plain" {
		t.Fatal("audit response body conversion failed")
	}

	response := httptest.NewRecorder()
	writeJSON(response, http.StatusCreated, map[string]string{"ok": "yes"})
	if response.Code != http.StatusCreated || response.Header().Get(headerContentType) != "application/json" {
		t.Fatalf("writeJSON response = %d, %#v", response.Code, response.Header())
	}
	response = httptest.NewRecorder()
	writeError(response, http.StatusBadRequest, "bad request", errors.New("internal detail"))
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "internal detail") {
		t.Fatalf("writeError response = %s", response.Body.String())
	}
}

func TestRecordMappersAndSmallHelpers(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	exitCode := 7
	run := runRecordFromStore(store.RunRecord{ID: "run-1", TaskID: "task-1", TaskVersionID: "version-1", ScheduleID: "schedule-1", ScheduleVersionID: "schedule-version-1", TaskName: "Task", State: "FAILED", TriggerType: "MANUAL", Runner: "runner-1", PlacementBlocker: "none", Attempt: 2, ExitCode: &exitCode, ExitCodeMeaning: "failure", Error: "failed", ScheduledFor: now, MaxMemoryUsedBytes: 10, AverageMemoryUsedBytes: 5})
	if run.ScheduledFor != now.Format(time.RFC3339) || run.ExitCode != &exitCode || run.Trigger != "MANUAL" {
		t.Fatalf("runRecordFromStore() = %+v", run)
	}
	task := taskRecordFromStore(store.TaskRecord{ID: "task-1", Name: "Task", Enabled: true, ActiveVersion: 2, RunnerPoolID: "pool-1", Command: []string{"echo", "ok"}, ResourceIDs: []string{"resource-1"}, LatestRun: &store.RunRecord{ID: "run-1", TaskID: "task-1"}})
	if task.Pool != "pool-1" || task.LatestRun == nil || !reflect.DeepEqual(task.Command, []string{"echo", "ok"}) {
		t.Fatalf("taskRecordFromStore() = %+v", task)
	}
	schedule := scheduleRecordFromStore(store.ScheduleRecord{ID: "schedule-1", Name: "Schedule", TaskID: "task-1", NextFireAt: &now, Enabled: true})
	if schedule.NextFireAt != now.Format(time.RFC3339) || !schedule.Enabled {
		t.Fatalf("scheduleRecordFromStore() = %+v", schedule)
	}
	runner := runnerRecordFromStore(store.RunnerRecord{ID: "runner-1", Name: "Runner", HeartbeatAt: &now, CurrentMetrics: &store.RunnerMetricsRecord{SampledAt: now, CPUPercent: 10}})
	if runner.HeartbeatAt != now.Format(time.RFC3339) || runner.CurrentMetrics == nil || runner.CurrentMetrics.CPUPercent != 10 {
		t.Fatalf("runnerRecordFromStore() = %+v", runner)
	}
	if got := runnerPoolRecordFromStore(store.RunnerPoolRecord{ID: "pool-1", Name: "Pool", Enabled: true}); got.ID != "pool-1" || !got.Enabled {
		t.Fatalf("runnerPoolRecordFromStore() = %+v", got)
	}
	if got := resourceRecordFromStore(store.ResourceRecord{ID: "resource-1", Name: "Resource", ExpiresAt: &now}); got.ExpiresAt != now.Format(time.RFC3339) {
		t.Fatalf("resourceRecordFromStore() = %+v", got)
	}

	if got := toAuthUser(store.UserRecord{Email: "alice@example.com", Enabled: true}); got.Status != store.StatusActive || !got.Enabled || got.DisplayName != "Alice" {
		t.Fatalf("toAuthUser() = %+v", got)
	}
	id, err := randomID()
	decoded, decodeErr := hex.DecodeString(id)
	if err != nil || decodeErr != nil || len(decoded) != 16 {
		t.Fatalf("randomID() = %q, %v", id, err)
	}
	if safeDeadLetterDiagnostic(strings.Repeat("x", deadLetterDiagnosticLimit+1)) != strings.Repeat("x", deadLetterDiagnosticLimit) || safeDeadLetterDiagnostic("token=hidden") != "[REDACTED]" {
		t.Fatal("dead-letter diagnostic was not bounded or redacted")
	}
	view := deadLetterView(store.DeadLetterSummary{ID: "dead-1", Error: "error", FirstFailedAt: now, LastFailedAt: now})
	if view.ID != "dead-1" || !view.FirstFailedAt.Equal(now) {
		t.Fatalf("deadLetterView() = %+v", view)
	}
	input, err := decodeDeadLetterAction(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"reason":" retry "}`)))
	if err != nil || input.Reason != "retry" {
		t.Fatalf("decodeDeadLetterAction() = %+v, %v", input, err)
	}
}

func TestInMemoryCollectionSuccessRoutes(t *testing.T) {
	request := func(path string) *http.Request { return httptest.NewRequest(http.MethodGet, path, nil) }
	assertOK := func(t *testing.T, name string, handler http.HandlerFunc, r *http.Request) {
		t.Helper()
		response := httptest.NewRecorder()
		handler(response, r)
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"items"`)) {
			t.Fatalf("%s response = %d %s", name, response.Code, response.Body.String())
		}
	}

	operations := NewOperationsService()
	operations.tasks["task-1"] = TaskRecord{ID: "task-1", Name: "Task"}
	operations.schedules["schedule-1"] = ScheduleRecord{ID: "schedule-1", Name: "Schedule"}
	assertOK(t, "tasks", operations.taskCollection, request("/api/v1/tasks"))
	assertOK(t, "schedules", operations.scheduleCollection, request("/api/v1/schedules"))

	runs := NewRunService()
	runs.runs["run-1"] = RunRecord{ID: "run-1", TaskID: "task-1"}
	assertOK(t, "runs", runs.collection, request("/api/v1/runs"))
	response := httptest.NewRecorder()
	runs.path(response, request("/api/v1/runs/run-1"))
	if response.Code != http.StatusOK {
		t.Fatalf("run detail response = %d", response.Code)
	}

	variables := NewGlobalVariableService()
	variables.items["variable-1"] = store.GlobalVariableRecord{ID: "variable-1", Name: "CACHE_PATH"}
	assertOK(t, "global variables", variables.collection, request("/api/v1/global-variables"))

	infrastructure := NewInfrastructureService()
	infrastructure.pools["pool-1"] = RunnerPoolRecord{ID: "pool-1", Name: "Pool"}
	infrastructure.runners["runner-1"] = RunnerRecord{ID: "runner-1", Name: "Runner"}
	infrastructure.resources["resource-1"] = ResourceRecord{ID: "resource-1", Name: "Resource"}
	assertOK(t, "pools", infrastructure.poolCollection, request("/api/v1/runners/pools"))
	assertOK(t, "runners", infrastructure.runnerCollection, request("/api/v1/runners"))
	assertOK(t, "resources", infrastructure.resourceCollection, request("/api/v1/resources"))
}
