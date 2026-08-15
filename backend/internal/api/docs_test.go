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
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Authorize with email and password") {
		t.Fatalf("docs response: %d", response.Code)
	}

	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("openapi response: %d", response.Code)
	}

	register := httptest.NewRecorder()
	h.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"docs-user@example.com","password":"correct horse"}`)))
	if register.Code != http.StatusCreated {
		t.Fatalf("register: %d", register.Code)
	}
	login := httptest.NewRecorder()
	h.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/docs/login", bytes.NewBufferString(`{"email":"docs-user@example.com","password":"correct horse"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("docs login: %d", login.Code)
	}
	if strings.Contains(login.Body.String(), "access_token") || !hasCookie(login.Result().Cookies(), accessCookie) {
		t.Fatalf("docs login response: %s", login.Body.String())
	}
}

func TestOpenAPICoversRegisteredAPIRoutes(t *testing.T) {
	var document struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &document); err != nil {
		t.Fatal(err)
	}
	for _, route := range RouteRegistry() {
		if !strings.HasPrefix(route.Pattern, "/api/v1") {
			continue
		}
		if !documentedRoute(document.Paths, route.Pattern) {
			t.Fatalf("route %s is missing from OpenAPI", route.Pattern)
		}
	}
	for path, operations := range document.Paths {
		route, ok := documentedRegistryRoute(path)
		if !ok {
			t.Fatalf("OpenAPI path %s is not registered", path)
		}
		for method, raw := range operations {
			if method == "parameters" || strings.HasPrefix(method, "x-") {
				continue
			}
			var operation struct {
				Security []json.RawMessage `json:"security"`
			}
			if err := json.Unmarshal(raw, &operation); err != nil {
				t.Fatalf("OpenAPI operation %s %s: %v", method, path, err)
			}
			if route.Access != RoutePublic && len(operation.Security) == 0 {
				t.Fatalf("protected route %s %s has no security scheme", method, path)
			}
		}
	}
}

func documentedRoute(paths map[string]map[string]json.RawMessage, pattern string) bool {
	if _, ok := paths[pattern]; ok {
		return true
	}
	if !strings.HasSuffix(pattern, "/") {
		return false
	}
	prefix := strings.TrimSuffix(pattern, "/") + "/"
	for path := range paths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func documentedRegistryRoute(path string) (RouteDefinition, bool) {
	for _, route := range RouteRegistry() {
		if route.Pattern == path || (strings.HasSuffix(route.Pattern, "/") && strings.HasPrefix(path, strings.TrimSuffix(route.Pattern, "/")+"/")) {
			return route, true
		}
	}
	return RouteDefinition{}, false
}
