package api

import (
	"bytes"
	"context"
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
	if strings.Contains(w.Body.String(), "access_token") || !hasCookie(w.Result().Cookies(), accessCookie) {
		t.Fatalf("login leaked or omitted session cookie: %s", w.Body.String())
	}
	var tokens AuthTokens
	for _, cookie := range w.Result().Cookies() {
		switch cookie.Name {
		case accessCookie:
			tokens.AccessToken = cookie.Value
		case refreshCookie:
			tokens.RefreshToken = cookie.Value
		case sessionCookie:
			tokens.SessionID = cookie.Value
		}
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
	if user, ok := auth.User(registered.ID); !ok || user.DisplayName != "Alice" {
		t.Fatalf("derived display name = %#v, %v", user, ok)
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

func TestSystemAdminReconciliationPreservesManualAssignments(t *testing.T) {
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
	admin, err := auth.Register("admin@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Grant(admin.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	second, err := auth.Register("second@example.com", "another correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Grant(second.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetSystemAdminEmails(nil); err != nil {
		t.Fatal(err)
	}
	roles, assignments, err := auth.roles.UserRoles(context.Background(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	adminRoleID := ""
	for _, role := range roles {
		if role.Name == "admin" {
			adminRoleID = role.ID
		}
	}
	manual, derived := false, false
	for _, assignment := range assignments {
		if assignment.RoleID != adminRoleID {
			continue
		}
		manual = manual || assignment.SourceType == "manual"
		derived = derived || assignment.SourceType == "system-admin"
	}
	if !manual || derived {
		t.Fatalf("reconciled assignments = %#v", assignments)
	}
	if err := auth.Revoke(admin.ID, "admin"); err != nil {
		t.Fatalf("manual administrator demotion failed: %v", err)
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
