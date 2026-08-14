package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuditQueryFiltersRedactsAndPaginates(t *testing.T) {
	audit := NewAuditQueryService()
	audit.Add(AuditEvent{ID: "old", Actor: "system:scheduler", Action: "run.created", Target: "run-1", Result: "success", CorrelationID: "corr-1", CreatedAt: "2026-08-13T10:00:00Z", Before: map[string]any{"token": "secret"}})
	audit.Add(AuditEvent{ID: "new", Actor: "user-1", Action: "user.updated", Target: "user-1", Result: "failure", CorrelationID: "corr-2", CreatedAt: "2026-08-14T10:00:00Z", After: map[string]any{"nested": map[string]any{"password": "secret"}}})
	audit.Add(AuditEvent{ID: "audit-read", Actor: "system:audit", Action: "GET", Target: "/api/v1/audit", Result: "success", CreatedAt: "2026-08-14T11:00:00Z"})
	if audit.events[0].Before["token"] != "[REDACTED]" || audit.events[1].After["nested"].(map[string]any)["password"] != "[REDACTED]" {
		t.Fatal("audit secrets were not redacted")
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/audit?actor=user-1&page=1&limit=1", nil)
	response := httptest.NewRecorder()
	audit.query(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"new"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"id":"old"`)) {
		t.Fatalf("filtered audit response: %d %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit?from="+time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC).Format(time.RFC3339), nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"id":"new"`)) || bytes.Contains(response.Body.Bytes(), []byte(`"id":"old"`)) {
		t.Fatalf("time-filtered audit response: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit?exclude_target=/api/v1/audit", nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if bytes.Contains(response.Body.Bytes(), []byte(`"id":"audit-read"`)) {
		t.Fatalf("excluded audit-read event returned: %s", response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	response = httptest.NewRecorder()
	audit.query(response, request)
	if !bytes.Contains(response.Body.Bytes(), []byte(`"id":"audit-read"`)) {
		t.Fatalf("audit-read event missing without exclusion: %s", response.Body.String())
	}
}

func TestAuditEventsIncludeUserActorDisplayData(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "users.read", "audit.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("actor@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	audit := NewAuditQueryService()
	server := Server{AuthService: auth, Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: user.ID}, true }, Permissions: auth.Permissions, AuditQuery: audit}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(audit.events) != 1 {
		t.Fatalf("user list audit: %d %d", response.Code, len(audit.events))
	}
	if audit.events[0].ActorName != user.Username || audit.events[0].ActorEmail != user.Email {
		t.Fatalf("actor display data: %#v", audit.events[0])
	}
}
