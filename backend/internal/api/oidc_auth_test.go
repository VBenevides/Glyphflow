package api

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestOIDCCallbackExchangesCodeAndIgnoresQueryClaims(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	oidc := NewOIDCService()
	var token string
	oidc.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body := []byte(`{"issuer":"https://issuer.example","token_endpoint":"https://issuer.example/token","jwks_uri":"https://issuer.example/jwks"}`)
		status := http.StatusOK
		if r.URL.Path == "/token" {
			content, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(content))
			if form.Get("code") != "code" || form.Get("code_verifier") == "" {
				status = http.StatusBadRequest
			} else {
				body, _ = json.Marshal(map[string]string{"id_token": token})
			}
		} else if r.URL.Path == "/jwks" {
			body = []byte(apiRSAJWKS(&key.PublicKey))
		} else if r.URL.Path != "/.well-known/openid-configuration" {
			status = http.StatusNotFound
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})})
	const issuer = "https://issuer.example"
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: issuer, Audience: "client", AuthURL: issuer + "/authorize", ClientID: "client", Callback: "https://app.example/callback", Enabled: true, AutoProvision: true}); err != nil {
		t.Fatal(err)
	}
	challenge, err := oidc.ChallengeWithPKCE("corp", "https://app.example/callback", now)
	if err != nil {
		t.Fatal(err)
	}
	token = apiSignOIDCToken(key, issuer, "client", challenge.Nonce, now.Add(time.Minute))
	auth, err := NewAuthService(strings.Repeat("x", 32), false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	if err := auth.AddRole("user", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	handler := (Server{AuthService: auth, OIDC: oidc}).Handler()
	query := url.Values{"state": {challenge.State}, "nonce": {challenge.Nonce}, "code": {"code"}, "subject": {"forged-subject"}, "issuer": {"https://forged.example"}}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/callback?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("callback status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var tokens AuthTokens
	if err := json.Unmarshal(recorder.Body.Bytes(), &tokens); err != nil {
		t.Fatal(err)
	}
	sessionRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	sessionRequest.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
	claims, ok := auth.sessions.Authenticator()(sessionRequest)
	if !ok {
		t.Fatal("callback did not issue an active session")
	}
	user, ok := auth.User(claims.UserID)
	if !ok || user.Email != "verified@example.com" {
		t.Fatalf("user = %#v, exists = %t", user, ok)
	}
	replay := httptest.NewRecorder()
	handler.ServeHTTP(replay, request)
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replayed callback status = %d", replay.Code)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func apiSignOIDCToken(key *rsa.PrivateKey, issuer, audience, nonce string, expiry time.Time) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"key-1"}`))
	payload, _ := json.Marshal(map[string]any{"iss": issuer, "sub": "verified-subject", "aud": audience, "nonce": nonce, "exp": expiry.Unix(), "preferred_username": "verified-user", "email": "verified@example.com"})
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(header + "." + encodedPayload))
	signature, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	return header + "." + encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func apiRSAJWKS(key *rsa.PublicKey) string {
	encode := func(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
	return `{"keys":[{"kty":"RSA","kid":"key-1","alg":"RS256","use":"sig","n":"` + encode(key.N.Bytes()) + `","e":"` + encode([]byte{1, 0, 1}) + `"}]}`
}
