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
