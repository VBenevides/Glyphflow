package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthServiceRegistrationLoginRefreshReplayAndPermissionRevocation(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	server := Server{AuthService: auth}
	h := server.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"Alice","password":"correct horse"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d", w.Code)
	}
	var registered struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &registered); err != nil || registered.ID == "" {
		t.Fatalf("registration response: %s", w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"alice","password":"correct horse"}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
	var tokens AuthTokens
	if err := json.Unmarshal(w.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.SessionID == "" {
		t.Fatal("login did not issue all session credentials")
	}
	refresh := tokens.RefreshToken
	next, err := auth.Refresh(tokens.SessionID, refresh)
	if err != nil || next.RefreshToken == refresh {
		t.Fatalf("refresh rotation failed: %#v %v", next, err)
	}
	if _, err := auth.Refresh(tokens.SessionID, refresh); err == nil {
		t.Fatal("refresh replay accepted")
	}
	claims, ok := auth.Authenticator()(httptest.NewRequest(http.MethodGet, "/", nil))
	if ok || claims.UserID != "" {
		t.Fatal("missing bearer token authenticated")
	}
	if !auth.Permissions(Claims{UserID: registered.ID})["tasks.read"] {
		t.Fatal("default role permission missing")
	}
}
