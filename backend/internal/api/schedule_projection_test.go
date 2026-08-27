package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/controlplane"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type scheduleProjectionRepositoryFunc func(context.Context) ([]store.ScheduleProjectionInput, error)

func (f scheduleProjectionRepositoryFunc) ListScheduleProjection(ctx context.Context) ([]store.ScheduleProjectionInput, error) {
	return f(ctx)
}

func TestScheduleProjectionEndpointReadsOnlyTheLatestSnapshot(t *testing.T) {
	calls := 0
	service := controlplane.NewProjectionService(scheduleProjectionRepositoryFunc(func(context.Context) ([]store.ScheduleProjectionInput, error) {
		calls++
		return []store.ScheduleProjectionInput{{ScheduleID: "schedule-1", TaskID: "task-1", TaskVersionID: "task-1-v1", Expression: "0 * * * *", Timezone: "UTC", RunnerPoolID: "pool-1", TimeoutSeconds: 60}}, nil
	}), nil)
	server := Server{
		Auth:               func(*http.Request) (Claims, bool) { return Claims{Roles: map[string]bool{"tasks.read": true}}, true },
		ScheduleProjection: service,
	}
	h := server.Handler()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/schedule-projection", nil))
	if response.Code != http.StatusOK || calls != 0 {
		t.Fatalf("unavailable response=%d calls=%d body=%s", response.Code, calls, response.Body.String())
	}
	var unavailable controlplane.ProjectionReport
	if err := json.Unmarshal(response.Body.Bytes(), &unavailable); err != nil {
		t.Fatal(err)
	}
	if unavailable.Available {
		t.Fatal("projection should be unavailable before its first refresh")
	}
	if err := service.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/schedule-projection", nil))
	if response.Code != http.StatusOK || calls != 1 {
		t.Fatalf("available response=%d calls=%d", response.Code, calls)
	}
	var available controlplane.ProjectionReport
	if err := json.Unmarshal(response.Body.Bytes(), &available); err != nil {
		t.Fatal(err)
	}
	if !available.Available || len(available.Segments) == 0 || !strings.Contains(response.Body.String(), `"calculatedAt"`) || strings.Contains(response.Body.String(), "command") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("projection response = %s", response.Body.String())
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/schedule-projection", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST returned %d", post.Code)
	}
}

func TestScheduleProjectionEndpointRequiresTaskRead(t *testing.T) {
	h := (Server{Auth: func(*http.Request) (Claims, bool) { return Claims{Roles: map[string]bool{}}, true }}).Handler()
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/schedule-projection", nil))
	if response.Code != http.StatusForbidden {
		t.Fatalf("forbidden response = %d", response.Code)
	}
}
