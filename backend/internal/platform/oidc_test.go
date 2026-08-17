package platform

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

func TestVerifyOIDCIDTokenRejectsForgedClaimsAndSignatures(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := rsaJWKS(&key.PublicKey, "key-1")
	tests := []struct {
		name   string
		issuer string
		aud    string
		nonce  string
		expiry time.Time
		signer *rsa.PrivateKey
		want   bool
	}{
		{"valid", "https://issuer.example", "client", "nonce", now.Add(time.Minute), key, true},
		{"wrong issuer", "https://forged.example", "client", "nonce", now.Add(time.Minute), key, false},
		{"wrong audience", "https://issuer.example", "other", "nonce", now.Add(time.Minute), key, false},
		{"wrong nonce", "https://issuer.example", "client", "other", now.Add(time.Minute), key, false},
		{"expired", "https://issuer.example", "client", "nonce", now.Add(-time.Minute), key, false},
		{"forged signature", "https://issuer.example", "client", "nonce", now.Add(time.Minute), other, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token := signOIDCTestToken(test.signer, test.issuer, test.aud, test.nonce, test.expiry)
			_, err := VerifyOIDCIDToken(token, jwks, "https://issuer.example", "client", "nonce", now)
			if (err == nil) != test.want {
				t.Fatalf("error = %v, want success = %t", err, test.want)
			}
		})
	}
}

func signOIDCTestToken(key *rsa.PrivateKey, issuer, audience, nonce string, expiry time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"key-1"}`))
	payload, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "subject", "aud": audience, "nonce": nonce, "exp": expiry.Unix()})
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(header + "." + encodedPayload))
	signature, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	return header + "." + encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func rsaJWKS(key *rsa.PublicKey, id string) string {
	encode := func(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
	return `{"keys":[{"kty":"RSA","kid":"` + id + `","alg":"RS256","use":"sig","n":"` + encode(key.N.Bytes()) + `","e":"` + encode(bigEndianInt(key.E)) + `"}]}`
}

func bigEndianInt(value int) []byte {
	return []byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)}
}
