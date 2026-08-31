package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

var errCode001SessionRepository = errors.New("session repository unavailable")

type code001FailingSessionRepository struct{ userID string }

func (code001FailingSessionRepository) Create(context.Context, store.SessionRecord) error {
	return errCode001SessionRepository
}

func (f code001FailingSessionRepository) Get(_ context.Context, id string) (store.SessionRecord, bool, error) {
	return store.SessionRecord{ID: id, UserID: f.userID, AccessExpiresAt: time.Now().Add(time.Hour)}, true, nil
}

func (code001FailingSessionRepository) Rotate(context.Context, string, string, store.SessionRecord) error {
	return errCode001SessionRepository
}

func (code001FailingSessionRepository) Active(context.Context, string, string) (bool, error) {
	return true, nil
}

func (code001FailingSessionRepository) Revoke(context.Context, string) error {
	return errCode001SessionRepository
}

func (code001FailingSessionRepository) RevokeFamily(context.Context, string) error {
	return errCode001SessionRepository
}

func (code001FailingSessionRepository) RevokeUser(context.Context, string) error {
	return errCode001SessionRepository
}

func (code001FailingSessionRepository) List(context.Context, string) ([]store.SessionRecord, error) {
	return nil, errCode001SessionRepository
}

func (code001FailingSessionRepository) DeleteOlderThan(context.Context, time.Time) error {
	return errCode001SessionRepository
}

func newCode001SessionRepositoryFailureHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("session-failure@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	auth.SetSessionRepository(code001FailingSessionRepository{userID: user.ID})
	claims := Claims{UserID: user.ID, SessionID: "session-1", Roles: map[string]bool{"users.read": true, "users.manage": true}}
	return Server{AuthService: auth, Auth: func(*http.Request) (Claims, bool) { return claims, true }}.Handler(), user.ID
}

func TestSessionRevokeReportsRepositoryFailure(t *testing.T) {
	handler, _ := newCode001SessionRepositoryFailureHandler(t)
	for _, path := range []string{
		"/api/v1/admin/auth/sessions/revoke?session_id=session-1",
		"/api/v1/me/sessions/revoke?session_id=session-1",
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"error":"session revoke failed"`) {
			t.Fatalf("POST %s returned %d %s", path, response.Code, response.Body.String())
		}
	}
}

func TestSessionRevokeAllReportsRepositoryFailure(t *testing.T) {
	handler, userID := newCode001SessionRepositoryFailureHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/users/"+userID+"/sessions/revoke-all", nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"error":"session revoke-all failed"`) {
		t.Fatalf("revoke all returned %d %s", response.Code, response.Body.String())
	}
}

func TestSessionListReportsRepositoryFailure(t *testing.T) {
	handler, _ := newCode001SessionRepositoryFailureHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/auth/sessions", nil))
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"error":"session list failed"`) || strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("list returned %d %s", response.Code, response.Body.String())
	}
}
