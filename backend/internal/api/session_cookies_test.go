package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCookieSessionLoginRefreshAndLogout(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	if _, err := auth.Register("user@example.com", "correct horse"); err != nil {
		t.Fatal(err)
	}
	handler := (Server{AuthService: auth}).Handler()
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"user@example.com","password":"correct horse"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d", login.Code)
	}
	cookies := login.Result().Cookies()
	if !hasCookie(cookies, accessCookie) || !hasCookie(cookies, refreshCookie) || !hasCookie(cookies, sessionCookie) {
		t.Fatalf("login did not set session cookies: %#v", cookies)
	}
	if strings.Contains(login.Body.String(), "access_token") || strings.Contains(login.Body.String(), "refresh_token") {
		t.Fatalf("login leaked token in JSON: %s", login.Body.String())
	}
	me := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	addCookies(me, cookies)
	profile := httptest.NewRecorder()
	handler.ServeHTTP(profile, me)
	if profile.Code != http.StatusOK {
		t.Fatalf("cookie profile status = %d", profile.Code)
	}
	refresh := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	addCookies(refresh, cookies)
	rotated := httptest.NewRecorder()
	handler.ServeHTTP(rotated, refresh)
	if rotated.Code != http.StatusOK {
		t.Fatalf("cookie refresh status = %d", rotated.Code)
	}
	cookies = rotated.Result().Cookies()
	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	addCookies(logout, cookies)
	loggedOut := httptest.NewRecorder()
	handler.ServeHTTP(loggedOut, logout)
	if loggedOut.Code != http.StatusNoContent {
		t.Fatalf("cookie logout status = %d", loggedOut.Code)
	}
	logoutCookies := loggedOut.Result().Cookies()
	if !hasExpiredCookie(logoutCookies, "glyphflow_csrf") {
		t.Fatalf("logout did not clear CSRF cookie: %#v", logoutCookies)
	}
	me = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	addCookies(me, cookies)
	profile = httptest.NewRecorder()
	handler.ServeHTTP(profile, me)
	if profile.Code != http.StatusUnauthorized {
		t.Fatalf("revoked cookie session returned %d", profile.Code)
	}
}

func TestSessionCookiesUseSecureFlagForHTTPSOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()
	(Server{CSRFOrigin: "https://console.example"}).setSessionCookies(recorder, AuthTokens{AccessToken: "access", RefreshToken: "refresh", SessionID: "session"})
	for _, cookie := range recorder.Result().Cookies() {
		if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
			t.Fatalf("insecure session cookie: %#v", cookie)
		}
	}
}

func TestSessionCookieDeletionUsesSecureFlagsForHTTPSOrigin(t *testing.T) {
	recorder := httptest.NewRecorder()
	(Server{CSRFOrigin: "https://console.example"}).clearSessionCookies(recorder)
	for _, cookie := range recorder.Result().Cookies() {
		if !cookie.Secure || !cookie.HttpOnly || cookie.MaxAge >= 0 {
			t.Fatalf("insecure cookie deletion: %#v", cookie)
		}
	}
}

func TestDisabledUserCannotUseExistingCookieSession(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("user@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	handler := (Server{AuthService: auth}).Handler()
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"user@example.com","password":"correct horse"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d", login.Code)
	}
	if err := auth.DisableUser(user.ID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	addCookies(request, login.Result().Cookies())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user session returned %d", response.Code)
	}
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value != "" && cookie.HttpOnly {
			return true
		}
	}
	return false
}

func hasExpiredCookie(cookies []*http.Cookie, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.MaxAge < 0 {
			return true
		}
	}
	return false
}

func addCookies(request *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		if cookie.Value != "" {
			request.AddCookie(cookie)
		}
	}
}
