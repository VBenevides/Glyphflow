package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOIDCProviderRepositoryRoundTrip(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := NewOIDCProviderRepository(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	id := "sso-test-" + suffix
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM sso_providers WHERE id = $1`, id) })
	provider := OIDCProviderRecord{ID: id, Name: "Corp-" + suffix, Issuer: "https://issuer.example", ClientID: "client", SecretReference: "secret://oidc/client", CallbackURLs: []string{"https://app.example/callback"}, AuthEndpointOverride: "https://issuer.example/authorize", Audience: "audience", Enabled: true, AutoProvision: true}
	if err := repository.Upsert(ctx, provider); err != nil {
		t.Fatal(err)
	}
	got, found, err := repository.Find(ctx, id)
	if err != nil || !found || got.SecretReference != provider.SecretReference || len(got.CallbackURLs) != 1 || got.CallbackURLs[0] != provider.CallbackURLs[0] {
		t.Fatalf("provider = %#v, found = %t, err = %v", got, found, err)
	}
	if err := repository.Upsert(ctx, provider); err != nil {
		t.Fatal(err)
	}
	duplicate := provider
	duplicate.ID = id + "-duplicate"
	duplicate.Name = "CORP-" + suffix
	if err := repository.Upsert(ctx, duplicate); err == nil {
		t.Fatal("case-insensitive provider name was accepted")
	}
	count, err := repository.EnabledCount(ctx)
	if err != nil || count < 1 {
		t.Fatalf("enabled provider count = %d, err = %v", count, err)
	}
}

func TestSSOIdentityAndGroupMappingRepositoryRoundTrip(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	providers := NewOIDCProviderRepository(pool)
	users := NewUserRepository(pool)
	roles := NewRoleRepository(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	providerID, userID, roleID := "sso-provider-"+suffix, "sso-user-"+suffix, "sso-role-"+suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
		_, _ = pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, roleID)
		_, _ = pool.Exec(ctx, `DELETE FROM sso_providers WHERE id = $1`, providerID)
	})
	if err := providers.Upsert(ctx, OIDCProviderRecord{ID: providerID, Name: providerID, Issuer: "https://issuer.example"}); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(ctx, UserRecord{ID: userID, Username: "sso-" + suffix + "@example.com", Email: "sso-" + suffix + "@example.com", Enabled: true}, ""); err != nil {
		t.Fatal(err)
	}
	if err := roles.Create(ctx, roleID, "sso-role-"+suffix, "", nil); err != nil {
		t.Fatal(err)
	}
	identity := SSOIdentityRecord{ID: "identity-" + suffix, UserID: userID, ProviderID: providerID, Subject: "subject"}
	if err := providers.CreateIdentity(ctx, identity); err != nil {
		t.Fatal(err)
	}
	got, found, err := providers.FindIdentity(ctx, providerID, identity.Subject)
	if err != nil || !found || got.UserID != userID {
		t.Fatalf("identity = %#v, found = %t, err = %v", got, found, err)
	}
	if err := providers.SetGroupRoleMapping(ctx, SSOGroupRoleMappingRecord{ProviderID: providerID, GroupName: "admins", RoleID: roleID}); err != nil {
		t.Fatal(err)
	}
	mappings, err := providers.ListGroupRoleMappings(ctx, providerID)
	if err != nil || len(mappings) != 1 || mappings[0].RoleID != roleID {
		t.Fatalf("mappings = %#v, err = %v", mappings, err)
	}
	if err := providers.DeleteGroupRoleMapping(ctx, providerID, "admins", roleID); err != nil {
		t.Fatal(err)
	}
	if err := providers.DeleteIdentity(ctx, userID, providerID, identity.Subject); err != nil {
		t.Fatal(err)
	}
}
