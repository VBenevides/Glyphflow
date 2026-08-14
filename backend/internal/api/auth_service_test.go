package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestAuthServiceRegistrationLoginRefreshReplayAndPermissionRevocation(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	server := Server{AuthService: auth}
	h := server.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString(`{"email":"Alice@example.com","password":"correct horse"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: %d", w.Code)
	}
	var registered struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &registered); err != nil || registered.ID == "" {
		t.Fatalf("registration response: %s", w.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewBufferString(`{"email":"alice@example.com","password":"correct horse"}`))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
	var tokens AuthTokens
	if err := json.Unmarshal(w.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" || tokens.SessionID == "" {
		t.Fatal("login did not issue all session credentials")
	}
	refresh := tokens.RefreshToken
	next, err := auth.Refresh(tokens.SessionID, refresh)
	if err != nil || next.RefreshToken == refresh {
		t.Fatalf("refresh rotation failed: %#v %v", next, err)
	}
	if _, err := auth.Refresh(tokens.SessionID, refresh); err == nil {
		t.Fatal("refresh replay accepted")
	}
	claims, ok := auth.Authenticator()(httptest.NewRequest(http.MethodGet, "/", nil))
	if ok || claims.UserID != "" {
		t.Fatal("missing bearer token authenticated")
	}
	if !auth.Permissions(Claims{UserID: registered.ID})["tasks.read"] {
		t.Fatal("default role permission missing")
	}
}

func TestAuthServiceProtectsLastAdministrator(t *testing.T) {
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
	first, err := auth.EnsureBootstrap("first@example.com", "correct horse", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.DisableUser(first.ID); !errors.Is(err, platform.ErrLastAdministrator) {
		t.Fatalf("last administrator disable returned %v", err)
	}
	second, err := auth.EnsureBootstrap("second@example.com", "correct horse 2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Grant(second.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := auth.DisableUser(first.ID); err != nil {
		t.Fatalf("administrator disable with a replacement returned %v", err)
	}
}

func TestSystemAdminEmailsGrantImmutableAdministrators(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("admin", "users.manage"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetSystemAdminEmails([]string{"Admin@Example.com", "second@example.com"}); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	admin, err := auth.Register("admin@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Permissions(Claims{UserID: admin.ID})["users.manage"] {
		t.Fatal("system admin was not granted admin permissions")
	}
	if err := auth.Revoke(admin.ID, "admin"); !errors.Is(err, platform.ErrSystemAdministrator) {
		t.Fatalf("system admin role revoke returned %v", err)
	}
	if err := auth.DisableUser(admin.ID); !errors.Is(err, platform.ErrSystemAdministrator) {
		t.Fatalf("system admin disable returned %v", err)
	}
	if _, err := auth.Register("ADMIN@example.com", "another correct horse"); err == nil {
		t.Fatal("duplicate email accepted")
	}
	if _, err := auth.Register("not-an-email", "another correct horse"); err == nil {
		t.Fatal("invalid email accepted")
	}
}

func TestPasswordFlagsBlockBackendEndpoints(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("alice@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.UpdateAuthSettings(false, false, "user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.ChangePassword(user.ID, "correct horse", "another correct horse"); err == nil {
		t.Fatal("disabled password login allowed password changes")
	}
	handler := (Server{AuthService: auth}).Handler()
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"alice@example.com","password":"correct horse"}`)))
	if login.Code != http.StatusUnauthorized {
		t.Fatalf("disabled password login returned %d", login.Code)
	}
	register := httptest.NewRecorder()
	handler.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"bob@example.com","password":"correct horse"}`)))
	if register.Code != http.StatusBadRequest {
		t.Fatalf("disabled password registration returned %d", register.Code)
	}
}
