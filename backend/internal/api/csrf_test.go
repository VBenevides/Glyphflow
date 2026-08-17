package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateCSRFRequest(t *testing.T) {
	token, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil)
	request.Header.Set("Origin", "https://console.example")
	request.Header.Set("X-CSRF-Token", token)
	request.AddCookie(&http.Cookie{Name: "glyphflow_csrf", Value: token})
	if err := ValidateCSRFRequest(request, []string{"https://console.example", "http://localhost:5173"}); err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://evil.example")
	if err := ValidateCSRFRequest(request, []string{"https://console.example", "http://localhost:5173"}); err == nil {
		t.Fatal("cross-site origin accepted")
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	if err := ValidateCSRFRequest(get, []string{"https://console.example", "http://localhost:5173"}); err != nil {
		t.Fatal(err)
	}
}
