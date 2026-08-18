package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationAdministrationRequiresPermission(t *testing.T) {
	password := NewPasswordAuthService(true, false, nil)
	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	admin := &AuthAdminService{Password: password, OIDC: oidc}
	server := Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"auth.settings.manage": true}}, true
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/settings", bytes.NewBufferString(`{"enabled":false}`))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK || password.enabled {
		t.Fatalf("settings update failed: %d", w.Code)
	}
	server.Auth = func(*http.Request) (Claims, bool) { return Claims{}, true }
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unpermissioned admin returned %d", w.Code)
	}
}

func TestAuthenticationAdministrationManagesSSOAndUsers(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("u@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	oidc := NewOIDCService()
	admin := &AuthAdminService{Auth: auth, OIDC: oidc}
	server := Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"sso.manage": true, "users.manage": true, "auth.settings.manage": true}}, true
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/providers", bytes.NewBufferString(`{"key":"corp","issuer":"https://issuer.example","callback":"https://app.example/callback","enabled":true}`))
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("provider create: %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/users/"+user.ID+"/disable", nil)
	w = httptest.NewRecorder()
	server.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("disable: %d", w.Code)
	}
	if got, _ := auth.User(user.ID); got.Enabled {
		t.Fatal("disabled user remains enabled")
	}
}

func TestAuthenticationAdministrationCreatesPromotesAndRevokesRoles(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"user", "admin", "operator"} {
		if err := auth.AddRole(role); err != nil {
			t.Fatal(err)
		}
	}
	auth.SetDefaultRole("user")
	server := Server{AuthAdmin: &AuthAdminService{Auth: auth}, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"users.manage": true}}, true
	}}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return response
	}
	created := request(http.MethodPost, "/api/v1/users", `{"email":"managed@example.com","password":"correct horse"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", created.Code, created.Body.String())
	}
	var user struct{ ID, Email string }
	if err := json.Unmarshal(created.Body.Bytes(), &user); err != nil || user.ID == "" || user.Email != "managed@example.com" {
		t.Fatalf("created user response: %s", created.Body.String())
	}
	if response := request(http.MethodPost, "/api/v1/admin/auth/users/"+user.ID+"/roles", `{"role":"operator"}`); response.Code != http.StatusNoContent {
		t.Fatalf("assign custom role: %d %s", response.Code, response.Body.String())
	}
	if response := request(http.MethodPost, "/api/v1/admin/auth/users/"+user.ID+"/roles", `{"role":"admin"}`); response.Code != http.StatusNoContent {
		t.Fatalf("promote admin: %d %s", response.Code, response.Body.String())
	}
	profile, ok := auth.UserProfile(user.ID)
	if !ok || !containsString(profile["roles"].([]string), "admin") || !containsString(profile["roles"].([]string), "operator") {
		t.Fatalf("assigned roles missing: %#v", profile)
	}
	if response := request(http.MethodDelete, "/api/v1/admin/auth/users/"+user.ID+"/roles/operator", ""); response.Code != http.StatusNoContent {
		t.Fatalf("revoke custom role: %d %s", response.Code, response.Body.String())
	}
	if response := request(http.MethodDelete, "/api/v1/admin/auth/users/"+user.ID+"/roles/admin", ""); response.Code != http.StatusConflict {
		t.Fatalf("last admin revoke: %d %s", response.Code, response.Body.String())
	}
}

func TestAuthenticationAdministrationFiltersAndRevokesSessions(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	alice, err := auth.Register("alice@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Register("bob@example.com", "correct horse 2"); err != nil {
		t.Fatal(err)
	}
	first, err := auth.Login(alice.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	second, err := auth.Login(alice.Email, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	server := Server{AuthAdmin: &AuthAdminService{Auth: auth}, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"users.read": true, "users.manage": true}}, true
	}}
	request := func(method, path string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(method, path, nil))
		return response
	}
	users := request(http.MethodGet, "/api/v1/users?email=alice@example.com&page=1&limit=10")
	if users.Code != http.StatusOK || !bytes.Contains(users.Body.Bytes(), []byte(alice.Email)) || bytes.Contains(users.Body.Bytes(), []byte("bob@example.com")) {
		t.Fatalf("filtered users: %d %s", users.Code, users.Body.String())
	}
	sessions := request(http.MethodGet, "/api/v1/admin/auth/sessions?email=alice@example.com&page=1&limit=10")
	if sessions.Code != http.StatusOK || !bytes.Contains(sessions.Body.Bytes(), []byte(first.SessionID)) || !bytes.Contains(sessions.Body.Bytes(), []byte(second.SessionID)) {
		t.Fatalf("filtered sessions: %d %s", sessions.Code, sessions.Body.String())
	}
	if response := request(http.MethodPost, "/api/v1/admin/auth/sessions/revoke?session_id="+first.SessionID); response.Code != http.StatusNoContent || auth.SessionManager().Owns(alice.ID, first.SessionID) {
		t.Fatalf("revoke session: %d", response.Code)
	}
	if response := request(http.MethodPost, "/api/v1/admin/auth/users/"+alice.ID+"/sessions/revoke-all"); response.Code != http.StatusNoContent || auth.SessionManager().Owns(alice.ID, second.SessionID) {
		t.Fatalf("revoke all sessions: %d", response.Code)
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestAuthenticationAdministrationReportsImmutableSystemAdmin(t *testing.T) {
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
	if err := auth.SetSystemAdminEmails([]string{"admin@example.com"}); err != nil {
		t.Fatal(err)
	}
	user, err := auth.EnsureBootstrap("admin@example.com", "correct horse", "", "")
	if err != nil {
		t.Fatal(err)
	}

	server := Server{AuthAdmin: &AuthAdminService{Auth: auth}, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"users.manage": true}}, true
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/users/"+user.ID+"/disable", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("immutable system admin returned %d", recorder.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] == "user not found" || body["error"] == "" {
		t.Fatalf("unexpected immutable admin error: %q", body["error"])
	}
	users := auth.Users()
	if len(users) != 1 || users[0]["systemAdmin"] != true {
		t.Fatalf("system admin flag missing: %#v", users)
	}
}

func TestAuthenticationAdministrationPreventsLastLoginMethodRemoval(t *testing.T) {
	password := NewPasswordAuthService(true, false, nil)
	admin := &AuthAdminService{Password: password}
	server := Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"auth.settings.manage": true}}, true
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/settings", bytes.NewBufferString(`{"enabled":false}`))
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("last password method removal returned %d", recorder.Code)
	}

	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	admin = &AuthAdminService{Password: NewPasswordAuthService(false, false, nil), OIDC: oidc}
	server = Server{AuthAdmin: admin, Auth: func(*http.Request) (Claims, bool) {
		return Claims{Roles: map[string]bool{"sso.manage": true}}, true
	}}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/providers", bytes.NewBufferString(`{"key":"corp","issuer":"https://issuer.example","callback":"https://app.example/callback","enabled":false}`))
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("last SSO method removal returned %d", recorder.Code)
	}
}
