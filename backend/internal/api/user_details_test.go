package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserDetailsAllowsSelfAndAdministratorsOnly(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("admin", "users.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	self, err := auth.Register("self@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.Register("other@example.com", "correct horse 2")
	if err != nil {
		t.Fatal(err)
	}
	current := self.ID
	handler := (Server{AuthService: auth, Auth: func(*http.Request) (Claims, bool) {
		return Claims{UserID: current}, true
	}}).Handler()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+current, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"roleSources"`)) {
		t.Fatalf("self details: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/users/"+other.ID, nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin details returned %d", response.Code)
	}

	if err := auth.Grant(self.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(other.Email)) {
		t.Fatalf("admin details: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/users/missing", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown details returned %d", response.Code)
	}
}
