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
	if strings.Contains(response.Body.String(), "unpkg.com") || !strings.Contains(response.Body.String(), "/openapi.json") {
		t.Fatalf("docs page is not offline-capable: %s", response.Body.String())
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

func TestDocsRejectUnsupportedMethods(t *testing.T) {
	server := Server{}
	for _, test := range []struct {
		path    string
		method  string
		handler http.HandlerFunc
	}{
		{path: "/docs", method: http.MethodPost, handler: swaggerUI},
		{path: "/openapi.json", method: http.MethodPost, handler: openAPI},
		{path: "/docs/login", method: http.MethodGet, handler: server.docsLogin},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, nil)
		test.handler(response, request)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s returned %d", test.path, response.Code)
		}
	}
}

func TestOpenAPICoversRegisteredAPIRoutes(t *testing.T) { // NOSONAR -- this invariant test intentionally checks every registered route and operation.
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

func TestOpenAPICanonicalizesImplementedAndDeprecatedOperations(t *testing.T) {
	var document struct {
		Paths map[string]map[string]struct {
			Deprecated bool                       `json:"deprecated"`
			Responses  map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &document); err != nil {
		t.Fatal(err)
	}
	for path, methodStatuses := range map[string]map[string]string{
		"/api/v1/tasks":        {"post": "201"},
		"/api/v1/schedules":    {"get": "200", "post": "201"},
		"/api/v1/runs":         {"get": "200"},
		"/api/v1/audit":        {"get": "200"},
		"/api/v1/runs/execute": {"post": "201"},
	} {
		for method, status := range methodStatuses {
			operation := document.Paths[path][method]
			if _, ok := operation.Responses[status]; !ok {
				t.Fatalf("OpenAPI %s %s is missing live status %s", method, path, status)
			}
		}
	}
	for path, methods := range map[string][]string{
		"/api/v1/roles": {"get", "post"}, "/api/v1/sso": {"get", "post"}, "/api/v1/logs": {"get"},
		"/api/v1/events": {"get"}, "/api/v1/runs/retry": {"post"}, "/api/v1/runs/cancel": {"post"},
	} {
		for _, method := range methods {
			operation := document.Paths[path][method]
			if !operation.Deprecated {
				t.Fatalf("legacy operation %s %s is not marked deprecated", method, path)
			}
			if _, ok := operation.Responses["410"]; !ok {
				t.Fatalf("legacy operation %s %s is missing 410 response", method, path)
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
