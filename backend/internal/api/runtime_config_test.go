package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeConfigPublishesFlagsAndIssuesCSRF(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	handler := (Server{AuthService: auth, OIDC: oidc, CSRFOrigin: "https://console.example"}).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"passwordLogin":true`) || !strings.Contains(response.Body.String(), `"oidc":true`) {
		t.Fatalf("config response: %d %s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "glyphflow_csrf" || cookies[0].Value == "" || cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("CSRF cookie: %#v", cookies)
	}
	blocked := httptest.NewRecorder()
	handler.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil))
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF request returned %d", blocked.Code)
	}
	checkedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil)
	checkedRequest.Header.Set("Origin", "https://console.example")
	checkedRequest.Header.Set("X-CSRF-Token", cookies[0].Value)
	checkedRequest.AddCookie(cookies[0])
	checked := httptest.NewRecorder()
	handler.ServeHTTP(checked, checkedRequest)
	if checked.Code != http.StatusUnauthorized {
		t.Fatalf("valid CSRF request returned %d", checked.Code)
	}
}
