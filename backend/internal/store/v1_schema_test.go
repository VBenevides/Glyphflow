package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestV1MigrationContainsIdentityAndVersionConstraints(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", "017_v1_identity_and_execution.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, fragment := range []string{"users_v1", "user_passwords_v1", "auth_sessions_v1", "roles_v1", "permissions_v1", "sso_providers_v1", "sso_authorization_states_v1", "task_versions_v1", "audit_events_v1", "unique (provider_id, provider_subject)"} {
		if !strings.Contains(sql, strings.ToLower(fragment)) {
			t.Fatalf("v1 migration missing %q", fragment)
		}
	}
}
