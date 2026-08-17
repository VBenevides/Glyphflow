package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionManagerRequiresStrongSecretAndActiveSession(t *testing.T) {
	if _, err := NewSessionManager("short"); err == nil {
		t.Fatal("weak access token secret accepted")
	}
	manager, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, claims, err := manager.Issue("user-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	got, ok := manager.Authenticator()(request)
	if !ok || got.UserID != claims.UserID || got.SessionID != claims.SessionID {
		t.Fatalf("active session rejected: %#v %v", got, ok)
	}
	manager.Revoke(claims.SessionID)
	if _, ok := manager.Authenticator()(request); ok {
		t.Fatal("revoked session accepted")
	}
}

func TestSessionManagerExpiresTokens(t *testing.T) {
	manager, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue("user-1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	for id, payload := range manager.sessions {
		payload.ExpiresAt = time.Now().Add(-time.Second).Unix()
		manager.sessions[id] = payload
	}
	manager.mu.Unlock()
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	if _, ok := manager.Authenticator()(request); ok {
		t.Fatal("expired session accepted")
	}
}
