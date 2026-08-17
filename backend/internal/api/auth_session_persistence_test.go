package api

import (
	"context"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuthServiceUsesDatabaseSessions(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL auth-session tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	auth, err := NewAuthService(strings.Repeat("x", 32), true, true, []byte("test-session-pepper"))
	if err != nil {
		t.Fatal(err)
	}
	auth.SetUserRepository(store.NewUserRepository(pool))
	auth.SetRoleRepository(store.NewRoleRepository(pool))
	auth.SetSessionRepository(store.NewSessionRepository(pool))
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SetDefaultRoleID("system-user"); err != nil {
		t.Fatal(err)
	}
	email := "session-auth-test@example.com"
	defer pool.Exec(ctx, `DELETE FROM users WHERE email = $1`, email)
	if _, err := auth.Register(email, "correct horse battery staple"); err != nil {
		t.Fatal(err)
	}
	tokens, err := auth.Login(email, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	if _, ok := auth.Authenticator()(request); !ok {
		t.Fatal("database-backed access token was rejected")
	}
	rotated, err := auth.Refresh(tokens.SessionID, tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := auth.Authenticator()(request); ok {
		t.Fatal("rotated access token remained active")
	}
	request.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	if _, ok := auth.Authenticator()(request); !ok {
		t.Fatal("rotated access token was rejected")
	}
	if _, err := auth.Refresh(tokens.SessionID, tokens.RefreshToken); err == nil {
		t.Fatal("refresh replay was accepted")
	}
	request.Header.Set("Authorization", "Bearer "+rotated.AccessToken)
	if _, ok := auth.Authenticator()(request); ok {
		t.Fatal("refresh replay did not revoke the session family")
	}
}
