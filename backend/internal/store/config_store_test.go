package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAuthenticationSettingsCommitAsOneTransaction(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL config tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	roleID := "config-test-role-" + suffix
	_, err = pool.Exec(ctx, `INSERT INTO roles (id, name) VALUES ($1, $2)`, roleID, "config-test-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
	config := NewConfigStore(pool)
	if err := config.SetAuthenticationSettings(ctx, true, true, roleID); err != nil {
		t.Fatal(err)
	}
	if err := config.SetAuthenticationSettings(ctx, false, false, "missing-role"); err == nil {
		t.Fatal("invalid default role was accepted")
	}
	var passwordLogin, registration bool
	var defaultRole string
	if found, err := config.Get(ctx, "ENABLE_PASSWORD_LOGIN", &passwordLogin); err != nil || !found || !passwordLogin {
		t.Fatalf("password login after rollback = %v, %v, %v", passwordLogin, found, err)
	}
	if found, err := config.Get(ctx, "ENABLE_PASSWORD_REGISTRATION", &registration); err != nil || !found || !registration {
		t.Fatalf("registration after rollback = %v, %v, %v", registration, found, err)
	}
	if found, err := config.Get(ctx, "DEFAULT_ROLE_ID", &defaultRole); err != nil || !found || defaultRole != roleID {
		t.Fatalf("default role after rollback = %q, %v, %v", defaultRole, found, err)
	}
}
