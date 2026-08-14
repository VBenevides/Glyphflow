package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserContractSmoke(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	handler := (Server{AuthService: auth, Auth: auth.Authenticator(), Permissions: auth.Permissions}).Handler()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Transport: handlerTransport{handler}}
	baseURL := "http://glyphflow.test"
	post := func(path, body string) *http.Response {
		t.Helper()
		response, requestErr := client.Post(baseURL+path, "application/json", strings.NewReader(body))
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		return response
	}
	response, err := client.Get(baseURL + "/api/v1/config")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Set-Cookie") == "" {
		t.Fatalf("config: %d", response.StatusCode)
	}
	response.Body.Close()

	response = post("/api/v1/auth/register", `{"email":"smoke@example.com","password":"correct horse"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("register: %d", response.StatusCode)
	}
	response.Body.Close()
	response = post("/api/v1/auth/login", `{"email":"smoke@example.com","password":"correct horse"}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login: %d", response.StatusCode)
	}
	response.Body.Close()

	response, err = client.Get(baseURL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || len(profile.Permissions) != 1 || profile.Permissions[0] != "tasks.read" {
		t.Fatalf("profile: %d %#v", response.StatusCode, profile)
	}

	response, err = client.Get(baseURL + "/api/v1/tasks")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("protected request: %d", response.StatusCode)
	}
	response.Body.Close()

	response = post("/api/v1/auth/refresh", "{}")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d", response.StatusCode)
	}
	response.Body.Close()
	response = post("/api/v1/auth/logout", "{}")
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: %d", response.StatusCode)
	}
	response.Body.Close()
	response, err = client.Get(baseURL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("revoked profile: %d %s", response.StatusCode, body)
	}
	response.Body.Close()
}

type handlerTransport struct{ handler http.Handler }

func (t handlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}
