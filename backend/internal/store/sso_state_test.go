package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOIDCAuthorizationStateRepositoryIsSingleUse(t *testing.T) {
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
	states := NewOIDCAuthorizationStateRepository(pool)
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	providerID := "state-provider-" + suffix
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM sso_providers WHERE id = $1`, providerID) })
	if err := providers.Upsert(ctx, OIDCProviderRecord{ID: providerID, Name: providerID, Issuer: "https://issuer.example", CallbackURLs: []string{"https://app.example/callback"}}); err != nil {
		t.Fatal(err)
	}
	hash := func(value string) string {
		digest := sha256.Sum256([]byte(value))
		return hex.EncodeToString(digest[:])
	}
	stateValue, nonceValue := "plaintext-state-"+suffix, "plaintext-nonce-"+suffix
	if err := states.Create(ctx, OIDCAuthorizationStateRecord{ID: "state-" + suffix, ProviderID: providerID, StateHash: hash(stateValue), NonceHash: hash(nonceValue), EncryptedPKCEVerifier: []byte("encrypted-verifier"), Purpose: "login", Callback: "https://app.example/callback", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	var storedState, storedNonce string
	var encrypted []byte
	if err := pool.QueryRow(ctx, `SELECT state_hash, nonce_hash, encrypted_pkce_verifier FROM sso_authorization_states WHERE id = $1`, "state-"+suffix).Scan(&storedState, &storedNonce, &encrypted); err != nil {
		t.Fatal(err)
	}
	if storedState == stateValue || storedNonce == nonceValue || strings.Contains(string(encrypted), "plaintext") {
		t.Fatal("authorization state was stored in plaintext")
	}
	if _, err := states.Consume(ctx, hash(stateValue), hash(nonceValue), providerID, "login", "https://app.example/callback", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := states.Consume(ctx, hash(stateValue), hash(nonceValue), providerID, "login", "https://app.example/callback", time.Now()); err == nil {
		t.Fatal("authorization state replay accepted")
	}
}
