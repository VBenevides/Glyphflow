package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdministrationAndAccountRoutes(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("alice", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	claims := Claims{UserID: user.ID, SessionID: "session-1", Roles: map[string]bool{"users.read": true, "users.manage": true, "auth.settings.manage": true}}
	h := (Server{AuthService: auth, Auth: func(*http.Request) (Claims, bool) { return claims, true }}).Handler()

	request := httptest.NewRequest(http.MethodPut, "/api/v1/me", bytes.NewBufferString(`{"display_name":"Alice Example"}`))
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("Alice Example")) {
		t.Fatalf("profile update: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/me/password", bytes.NewBufferString(`{"current_password":"correct horse","new_password":"new correct horse"}`))
	response = httptest.NewRecorder()
	h.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("password update: %d", response.Code)
	}
	if _, err := auth.Login("alice", "new correct horse"); err != nil {
		t.Fatal("new password was not applied")
	}

	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/users?page=1", nil))
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(user.ID)) {
		t.Fatalf("user list: %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/settings", bytes.NewBufferString(`{"enabled":true,"registration":false,"default_role":"user"}`)))
	if response.Code != http.StatusOK || auth.RegistrationEnabled() {
		t.Fatalf("settings update: %d", response.Code)
	}

	if err := auth.LinkOIDC(user.ID, "corp", "subject"); err != nil {
		t.Fatal(err)
	}
	profile := auth.Profile(claims)
	identities, ok := profile["identities"].([]map[string]any)
	if !ok || len(identities) != 1 {
		encoded, _ := json.Marshal(profile)
		t.Fatalf("profile identities: %s", encoded)
	}
	id := identities[0]["id"].(string)
	response = httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/v1/me/identities/"+id, nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("identity unlink: %d", response.Code)
	}
}
