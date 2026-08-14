package integration

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/api"
	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestAuthenticationModesAndImmediatePermissionChanges(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		sessions, _ := api.NewSessionManager("01234567890123456789012345678901")
		password := api.NewPasswordAuthService(enabled, enabled, nil)
		server := api.Server{PasswordAuth: password, Sessions: sessions, Auth: sessions.Authenticator()}
		register := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"username":"u","password":"password"}`))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, register)
		if enabled && response.Code != http.StatusCreated {
			t.Fatalf("password mode registration: %d", response.Code)
		}
		if !enabled && response.Code == http.StatusCreated {
			t.Fatal("password-disabled mode registered a user")
		}
	}
	seed, err := platform.SeedRoles()
	if err != nil || len(seed) != 2 || !seed[0].System {
		t.Fatalf("seed catalog: %#v %v", seed, err)
	}
	granted := false
	server := api.Server{Auth: func(*http.Request) (api.Claims, bool) { return api.Claims{UserID: "u"}, true }, Permissions: func(api.Claims) map[string]bool {
		if granted {
			return map[string]bool{"tasks.read": true}
		}
		return nil
	}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("permission unexpectedly granted: %d", response.Code)
	}
	granted = true
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code == http.StatusForbidden {
		t.Fatal("permission grant did not apply")
	}
}
