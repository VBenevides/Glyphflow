package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestOIDCProviderChallengeIsSingleUse(t *testing.T) {
	s := NewOIDCService()
	if err := s.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://id.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	link, err := s.Challenge("corp", "https://app.example/callback", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := http.NewRequest(http.MethodGet, link, nil)
	q := parsed.URL.Query()
	if err := s.Complete("corp", q.Get("state"), q.Get("nonce"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete("corp", q.Get("state"), q.Get("nonce"), time.Now()); err == nil {
		t.Fatal("OIDC state replay accepted")
	}
	_ = httptest.NewRecorder()
}

func TestOIDCChallengeUsesPKCES256AndCompletesLogin(t *testing.T) {
	s := NewOIDCService()
	if err := s.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Audience: "client", AuthURL: "https://issuer.example/authorize", ClientID: "client", Callback: "https://app.example/callback", Enabled: true, AutoProvision: true}); err != nil {
		t.Fatal(err)
	}
	challenge, err := s.ChallengeWithPKCE("corp", "https://app.example/callback", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := http.NewRequest(http.MethodGet, challenge.URL, nil)
	if parsed.URL.Query().Get("code_challenge_method") != "S256" || parsed.URL.Query().Get("code_challenge") == "" {
		t.Fatal("PKCE S256 parameters missing")
	}
	auth, err := NewAuthService("01234567890123456789012345678901", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	claims := platform.OIDCClaims{Issuer: "https://issuer.example", Subject: "subject", Audience: []string{"client"}, Nonce: challenge.Nonce, Expires: time.Now().Add(time.Minute)}
	if err := s.CompleteChallenge("corp", challenge.State, challenge.Nonce, "https://app.example/callback", challenge.Verifier, time.Now(), claims); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.LoginOIDC("corp", "subject", "alice", "alice@example.com", true); err != nil {
		t.Fatal(err)
	}
}
