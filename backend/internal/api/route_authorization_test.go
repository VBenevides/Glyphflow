package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRouteAuthorizationCoverage(t *testing.T) {
	if err := ValidateRouteRegistry(RouteRegistry()); err != nil {
		t.Fatal(err)
	}
	for _, route := range RouteRegistry() {
		if route.Access != RoutePermission {
			continue
		}
		permission := strings.Split(route.Permission, "|")[0]
		server := Server{AuthAdmin: &AuthAdminService{Password: NewPasswordAuthService(true, false, nil)}, Roles: NewRoleAdminService(), Auth: func(*http.Request) (Claims, bool) { return Claims{Roles: map[string]bool{}}, true }}
		path := strings.TrimSuffix(route.Pattern, "/")
		if strings.HasSuffix(route.Pattern, "/") {
			path += "/test/disable"
		}
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden && response.Code != http.StatusNotImplemented {
			t.Fatalf("%s denied with %d", route.Pattern, response.Code)
		}
		server.Auth = func(*http.Request) (Claims, bool) {
			return Claims{Roles: map[string]bool{permission: true, "run.retry": true, "run.cancel": true, "task.read": true}}, true
		}
		response = httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code == http.StatusForbidden || response.Code == http.StatusUnauthorized {
			t.Fatalf("%s allowed request denied with %d", route.Pattern, response.Code)
		}
	}
}
