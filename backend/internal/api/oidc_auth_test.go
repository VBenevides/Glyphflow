package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
