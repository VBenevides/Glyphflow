package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestPasswordEndpointsRegisterAndLogin(t *testing.T) {
	sessions, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	server := Server{PasswordAuth: NewPasswordAuthService(true, true, nil), Sessions: sessions}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"user","password":"correct horse"}`))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"user","password":"correct horse"}`))
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
}

func TestAuthenticationEndpointsRateLimitByIdentityAndSource(t *testing.T) {
	sessions, err := NewSessionManager("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	server := Server{PasswordAuth: NewPasswordAuthService(true, true, nil), Sessions: sessions, AuthRateLimiter: platform.NewRateLimiter(1, time.Minute)}
	login := func(username string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"username":"`+username+`","password":"wrong"}`))
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		return recorder
	}
	if login("user").Code != http.StatusUnauthorized || login("user").Code != http.StatusTooManyRequests {
		t.Fatal("password attempts were not rate limited")
	}
	if login("other-user").Code != http.StatusUnauthorized {
		t.Fatal("password rate limit leaked between usernames")
	}

	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	oidcServer := Server{OIDC: oidc, AuthRateLimiter: platform.NewRateLimiter(1, time.Minute)}
	challengeURL := "/api/v1/auth/oidc/login?provider=corp&redirect_uri=https%3A%2F%2Fapp.example%2Fcallback"
	first := httptest.NewRecorder()
	oidcServer.Handler().ServeHTTP(first, httptest.NewRequest(http.MethodGet, challengeURL, nil))
	second := httptest.NewRecorder()
	oidcServer.Handler().ServeHTTP(second, httptest.NewRequest(http.MethodGet, challengeURL, nil))
	if first.Code != http.StatusFound || second.Code != http.StatusTooManyRequests {
		t.Fatalf("OIDC challenge statuses = %d, %d", first.Code, second.Code)
	}
}
