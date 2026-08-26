package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

func TestPendingApprovalLifecycleAndSettings(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"user", "admin"} {
		if err := auth.AddRole(role); err != nil {
			t.Fatal(err)
		}
	}
	auth.SetDefaultRole("user")
	handler := (Server{
		AuthService: auth,
		AuthAdmin:   &AuthAdminService{Auth: auth},
		Auth: func(*http.Request) (Claims, bool) {
			return Claims{Roles: map[string]bool{"auth.settings.manage": true, "users.manage": true}}, true
		},
	}).Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return response
	}

	settings := request(http.MethodPost, "/api/v1/admin/auth/settings", `{"require_user_approval":true}`)
	if settings.Code != http.StatusOK || !auth.UserApprovalRequired() || !strings.Contains(settings.Body.String(), `"require_user_approval":true`) {
		t.Fatalf("enable approval: %d %s", settings.Code, settings.Body.String())
	}
	runtime := request(http.MethodGet, "/api/v1/config", "")
	if runtime.Code != http.StatusOK || !strings.Contains(runtime.Body.String(), `"requireUserApproval":true`) {
		t.Fatalf("runtime approval setting: %d %s", runtime.Code, runtime.Body.String())
	}
	registered := request(http.MethodPost, "/api/v1/auth/register", `{"email":"pending@example.com","password":"correct horse"}`)
	if registered.Code != http.StatusCreated || !strings.Contains(registered.Body.String(), `"status":"pending"`) {
		t.Fatalf("self-registration status: %d %s", registered.Code, registered.Body.String())
	}
	login := request(http.MethodPost, "/api/v1/auth/login", `{"email":"pending@example.com","password":"correct horse"}`)
	if login.Code != http.StatusForbidden || !strings.Contains(login.Body.String(), "pending administrator approval") {
		t.Fatalf("pending login: %d %s", login.Code, login.Body.String())
	}
	created := request(http.MethodPost, "/api/v1/users", `{"email":"managed@example.com","password":"correct horse 2"}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"status":"pending"`) {
		t.Fatalf("admin-created status: %d %s", created.Code, created.Body.String())
	}
	var managed struct{ ID string }
	if err := json.Unmarshal(created.Body.Bytes(), &managed); err != nil || managed.ID == "" {
		t.Fatalf("admin-created response: %v %s", err, created.Body.String())
	}
	if response := request(http.MethodPost, "/api/v1/admin/auth/users/"+managed.ID+"/approve", ""); response.Code != http.StatusNoContent {
		t.Fatalf("approve: %d %s", response.Code, response.Body.String())
	}
	if user, ok := auth.User(managed.ID); !ok || user.Status != store.StatusActive || !user.Enabled {
		t.Fatalf("approved user: %#v %v", user, ok)
	}
	if response := request(http.MethodPost, "/api/v1/auth/login", `{"email":"managed@example.com","password":"correct horse 2"}`); response.Code != http.StatusOK {
		t.Fatalf("approved login: %d %s", response.Code, response.Body.String())
	}
	if response := request(http.MethodPost, "/api/v1/admin/auth/users/"+managed.ID+"/disable", ""); response.Code != http.StatusNoContent {
		t.Fatalf("disable: %d %s", response.Code, response.Body.String())
	}
	if user, _ := auth.User(managed.ID); user.Status != store.StatusDisabled {
		t.Fatalf("disabled user: %#v", user)
	}
	if response := request(http.MethodPost, "/api/v1/admin/auth/users/"+managed.ID+"/approve", ""); response.Code != http.StatusNoContent {
		t.Fatalf("re-enable: %d %s", response.Code, response.Body.String())
	}
	if response := request(http.MethodPost, "/api/v1/auth/login", `{"email":"managed@example.com","password":"correct horse 2"}`); response.Code != http.StatusOK {
		t.Fatalf("re-enabled login: %d %s", response.Code, response.Body.String())
	}
	settings = request(http.MethodPost, "/api/v1/admin/auth/settings", `{"require_user_approval":false}`)
	if settings.Code != http.StatusOK || auth.UserApprovalRequired() || !strings.Contains(settings.Body.String(), `"require_user_approval":false`) {
		t.Fatalf("disable approval: %d %s", settings.Code, settings.Body.String())
	}
	active, err := auth.Register("active@example.com", "correct horse 3")
	if err != nil || active.Status != store.StatusActive {
		t.Fatalf("registration without approval: %#v %v", active, err)
	}
}

func TestPendingApprovalDoesNotAffectBootstrap(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("admin"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	auth.SetUserApprovalRequired(true)
	bootstrap, err := auth.EnsureBootstrap("bootstrap@example.com", "correct horse", "", "")
	if err != nil || bootstrap.Status != store.StatusActive || !bootstrap.Enabled {
		t.Fatalf("bootstrap status: %#v %v", bootstrap, err)
	}
	if _, err := auth.Login("bootstrap@example.com", "correct horse"); err != nil {
		t.Fatalf("bootstrap login: %v", err)
	}
	if _, err := auth.Login("missing@example.com", "correct horse"); err == nil || errors.Is(err, ErrPendingUser) {
		t.Fatalf("missing login error: %v", err)
	}
}
