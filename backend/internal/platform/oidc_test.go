package platform

import (
	"testing"
	"time"
)

func TestOIDCValidationRequiresIssuerAudienceNonceAndHTTPSCallback(t *testing.T) {
	now := time.Now()
	claims := OIDCClaims{Issuer: "https://issuer.example", Subject: "subject", Audience: []string{"glyphflow"}, Nonce: "nonce", Expires: now.Add(time.Minute)}
	if err := ValidateOIDCClaims(claims, "https://issuer.example", "glyphflow", "nonce", now); err != nil {
		t.Fatal(err)
	}
	claims.Nonce = "other"
	if err := ValidateOIDCClaims(claims, "https://issuer.example", "glyphflow", "nonce", now); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	if err := ValidateOIDCCallback("http://issuer.example/callback", "https://issuer.example/callback"); err == nil {
		t.Fatal("HTTP callback accepted")
	}
	if err := ValidateOIDCCallback("https://issuer.example/callback", "https://issuer.example/callback"); err != nil {
		t.Fatal(err)
	}
	if PKCEChallenge("verifier") == PKCEChallenge("other") {
		t.Fatal("PKCE challenge collision")
	}
}
