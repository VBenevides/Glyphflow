package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type OIDCProvider struct {
	Key, Issuer, ClientID, Callback, AuthURL, Audience string
	Enabled, AutoProvision                             bool
	Callbacks                                          []string
}
type OIDCService struct {
	mu         sync.RWMutex
	providers  map[string]OIDCProvider
	states     *platform.AuthorizationStateStore
	httpClient *http.Client
}

func NewOIDCService() *OIDCService {
	return &OIDCService{providers: map[string]OIDCProvider{}, states: platform.NewAuthorizationStateStore(), httpClient: &http.Client{Timeout: 10 * time.Second}}
}

func (s *OIDCService) SetHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	s.mu.Lock()
	s.httpClient = client
	s.mu.Unlock()
}
func (s *OIDCService) AddProvider(provider OIDCProvider) error {
	if provider.Key == "" || provider.Issuer == "" || provider.Callback == "" {
		return errors.New("OIDC provider is incomplete")
	}
	if _, err := secureURL(provider.Issuer); err != nil {
		return err
	}
	callbacks := provider.Callbacks
	if len(callbacks) == 0 {
		callbacks = []string{provider.Callback}
	}
	for _, callback := range callbacks {
		if err := platform.ValidateOIDCCallback(callback, callback); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.providers[provider.Key] = provider
	s.mu.Unlock()
	return nil
}
func (s *OIDCService) Providers() []OIDCProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []OIDCProvider
	for _, p := range s.providers {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out
}

func (s *OIDCService) Provider(key string) (OIDCProvider, bool) {
	return s.provider(key)
}

func (s *OIDCService) EnabledCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, provider := range s.providers {
		if provider.Enabled {
			count++
		}
	}
	return count
}
func (s *OIDCService) Challenge(key, redirect string, now time.Time) (string, error) {
	challenge, err := s.ChallengeWithPKCE(key, redirect, now)
	if err != nil {
		return "", err
	}
	return challenge.URL, nil
}

type OIDCChallenge struct{ URL, State, Nonce, Verifier string }

func (s *OIDCService) ChallengeWithPKCE(key, redirect string, now time.Time) (OIDCChallenge, error) {
	s.mu.RLock()
	p, ok := s.providers[key]
	s.mu.RUnlock()
	if !ok || !p.Enabled {
		return OIDCChallenge{}, errors.New("OIDC provider is unavailable")
	}
	allowed := false
	callbacks := p.Callbacks
	if len(callbacks) == 0 {
		callbacks = []string{p.Callback}
	}
	for _, callback := range callbacks {
		if callback == redirect {
			allowed = true
		}
	}
	if !allowed {
		return OIDCChallenge{}, errors.New("OIDC callback is not configured")
	}
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return OIDCChallenge{}, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	state, nonce, err := s.states.CreateChallenge(key, "login", redirect, verifier, now, 10*time.Minute)
	if err != nil {
		return OIDCChallenge{}, err
	}
	endpoint := p.AuthURL
	if endpoint == "" {
		endpoint = p.Issuer
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return OIDCChallenge{}, err
	}
	q := u.Query()
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("redirect_uri", redirect)
	q.Set("code_challenge", platform.PKCEChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("response_type", "code")
	q.Set("client_id", p.ClientID)
	u.RawQuery = q.Encode()
	return OIDCChallenge{URL: u.String(), State: state, Nonce: nonce, Verifier: verifier}, nil
}
func (s *OIDCService) Complete(key, state, nonce string, now time.Time) error {
	return s.states.Consume(state, nonce, key, "login", now)
}

func (s *OIDCService) CompleteChallenge(key, state, nonce, callback, verifier string, now time.Time, claims platform.OIDCClaims) error {
	if err := s.states.ConsumeChallenge(state, nonce, key, "login", callback, verifier, now); err != nil {
		return err
	}
	s.mu.RLock()
	provider, ok := s.providers[key]
	s.mu.RUnlock()
	if !ok {
		return errors.New("OIDC provider is unavailable")
	}
	return platform.ValidateOIDCClaims(claims, provider.Issuer, provider.Audience, nonce, now)
}

func (s *OIDCService) CompleteAuthorizationCode(state, nonce, code string, now time.Time) (OIDCProvider, platform.OIDCClaims, error) {
	if code == "" {
		return OIDCProvider{}, platform.OIDCClaims{}, errors.New("OIDC authorization code is required")
	}
	key, callback, verifier, err := s.states.ReadChallenge(state, nonce, "login", now)
	if err != nil {
		return OIDCProvider{}, platform.OIDCClaims{}, err
	}
	provider, ok := s.provider(key)
	if !ok || !provider.Enabled {
		return OIDCProvider{}, platform.OIDCClaims{}, errors.New("OIDC provider is unavailable")
	}
	metadata, err := s.discovery(provider)
	if err != nil {
		return OIDCProvider{}, platform.OIDCClaims{}, err
	}
	token, err := s.exchangeCode(provider, metadata.TokenEndpoint, code, callback, verifier)
	if err != nil {
		return OIDCProvider{}, platform.OIDCClaims{}, err
	}
	jwks, err := s.fetch(metadata.JWKSURI, nil)
	if err != nil {
		return OIDCProvider{}, platform.OIDCClaims{}, err
	}
	audience := provider.Audience
	if audience == "" {
		audience = provider.ClientID
	}
	claims, err := platform.VerifyOIDCIDToken(token, string(jwks), metadata.Issuer, audience, nonce, now)
	if err != nil {
		return OIDCProvider{}, platform.OIDCClaims{}, err
	}
	if err := s.states.ConsumeChallenge(state, nonce, key, "login", callback, verifier, now); err != nil {
		return OIDCProvider{}, platform.OIDCClaims{}, err
	}
	return provider, claims, nil
}

type oidcMetadata struct {
	Issuer        string `json:"issuer"`
	TokenEndpoint string `json:"token_endpoint"`
	JWKSURI       string `json:"jwks_uri"`
}

func (s *OIDCService) discovery(provider OIDCProvider) (oidcMetadata, error) {
	issuer, err := secureURL(provider.Issuer)
	if err != nil {
		return oidcMetadata{}, err
	}
	body, err := s.fetch(strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return oidcMetadata{}, err
	}
	var metadata oidcMetadata
	if err := json.Unmarshal(body, &metadata); err != nil || metadata.Issuer != provider.Issuer {
		return oidcMetadata{}, errors.New("OIDC discovery issuer is invalid")
	}
	if _, err := secureURL(metadata.TokenEndpoint); err != nil {
		return oidcMetadata{}, err
	}
	if _, err := secureURL(metadata.JWKSURI); err != nil {
		return oidcMetadata{}, err
	}
	return metadata, nil
}

func (s *OIDCService) exchangeCode(provider OIDCProvider, endpoint, code, callback, verifier string) (string, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "client_id": {provider.ClientID}, "code_verifier": {verifier}}
	body, err := s.fetch(endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	var response struct {
		IDToken string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &response); err != nil || response.IDToken == "" {
		return "", errors.New("OIDC token response has no ID token")
	}
	return response.IDToken, nil
}

func (s *OIDCService) fetch(endpoint string, body io.Reader) ([]byte, error) {
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
	}
	s.mu.RLock()
	client := s.httpClient
	s.mu.RUnlock()
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("OIDC provider request failed")
	}
	var value json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func secureURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("OIDC endpoint must use HTTPS")
	}
	return parsed.String(), nil
}

func (s Server) oidcRoutes(mux *http.ServeMux) {
	if s.OIDC == nil {
		return
	}
	mux.HandleFunc("/api/v1/auth/oidc/providers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		writeJSON(w, 200, s.OIDC.Providers())
	})
	mux.HandleFunc("/api/v1/auth/oidc/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		if !s.allowAuth(w, r, "oidc-challenge|"+platform.NormalizeIdentityKey(r.URL.Query().Get("provider"))) {
			return
		}
		redirect, err := s.OIDC.Challenge(r.URL.Query().Get("provider"), r.URL.Query().Get("redirect_uri"), time.Now())
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "OIDC challenge failed"})
			return
		}
		http.Redirect(w, r, redirect, http.StatusFound)
	})
	mux.HandleFunc("/api/v1/auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, 405, map[string]string{"error": "method not allowed"})
			return
		}
		if !s.allowAuth(w, r, "oidc-callback") {
			return
		}
		if s.AuthService == nil {
			writeJSON(w, 503, map[string]string{"error": "authentication unavailable"})
			return
		}
		now := time.Now()
		provider, claims, err := s.OIDC.CompleteAuthorizationCode(r.URL.Query().Get("state"), r.URL.Query().Get("nonce"), r.URL.Query().Get("code"), now)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "OIDC callback failed"})
			return
		}
		tokens, err := s.AuthService.LoginOIDC(provider.Key, claims.Subject, claims.Username, claims.Email, provider.AutoProvision)
		if err != nil {
			writeJSON(w, 401, map[string]string{"error": "OIDC login failed"})
			return
		}
		writeJSON(w, 200, tokens)
	})
}

func (s *OIDCService) provider(key string) (OIDCProvider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[key]
	return p, ok
}
