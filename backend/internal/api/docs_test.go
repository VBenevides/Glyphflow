package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDocsAndPasswordAuthorization(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	h := (Server{AuthService: auth}).Handler()

	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/docs", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Authorize with username and password") {
		t.Fatalf("docs response: %d", response.Code)
	}

	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("openapi response: %d", response.Code)
	}

	register := httptest.NewRecorder()
	h.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"docs-user","password":"correct horse"}`)))
	if register.Code != http.StatusCreated {
		t.Fatalf("register: %d", register.Code)
	}
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/docs/login", bytes.NewBufferString(`{"username":"docs-user","password":"correct horse"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("docs login: %d", login.Code)
	}
	var tokens AuthTokens
	if err := json.Unmarshal(login.Body.Bytes(), &tokens); err != nil || tokens.AccessToken == "" {
		t.Fatalf("docs login response: %s", login.Body.String())
	}
}
