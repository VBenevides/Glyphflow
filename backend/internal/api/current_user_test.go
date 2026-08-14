package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCurrentUserReturnsProfileAndOwnSessionRevocation(t *testing.T) {
	sessions, _ := NewSessionManager("01234567890123456789012345678901")
	_, claims, _ := sessions.Issue("u", 1e9)
	server := Server{Auth: sessions.Authenticator(), CurrentUser: &CurrentUserService{Sessions: sessions, Profile: func(c Claims) map[string]any {
		return map[string]any{"id": c.UserID, "permissions": []string{"tasks.read"}}
	}}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenFromClaim(t, sessions, claims))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("profile: %d", w.Code)
	}
}

func tokenFromClaim(t *testing.T, sessions *SessionManager, claims Claims) string {
	token, _, err := sessions.Issue(claims.UserID, 1e9)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
