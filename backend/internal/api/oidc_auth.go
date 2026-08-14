package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/url"
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
	mu        sync.RWMutex
	providers map[string]OIDCProvider
	states    *platform.AuthorizationStateStore
}

func NewOIDCService() *OIDCService {
	return &OIDCService{providers: map[string]OIDCProvider{}, states: platform.NewAuthorizationStateStore()}
}
func (s *OIDCService) AddProvider(provider OIDCProvider) error {
	if provider.Key == "" || provider.Issuer == "" || provider.Callback == "" {
		return errors.New("OIDC provider is incomplete")
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
		if s.AuthService == nil {
			writeJSON(w, 503, map[string]string{"error": "authentication unavailable"})
			return
		}
		key := r.URL.Query().Get("provider")
		now := time.Now()
		claims := platform.OIDCClaims{Issuer: r.URL.Query().Get("issuer"), Subject: r.URL.Query().Get("subject"), Audience: []string{r.URL.Query().Get("audience")}, Nonce: r.URL.Query().Get("nonce")}
		if expiry := r.URL.Query().Get("expires"); expiry != "" {
			if parsed, err := time.Parse(time.RFC3339, expiry); err == nil {
				claims.Expires = parsed
			}
		}
		if err := s.OIDC.CompleteChallenge(key, r.URL.Query().Get("state"), r.URL.Query().Get("nonce"), r.URL.Query().Get("redirect_uri"), r.URL.Query().Get("verifier"), now, claims); err != nil {
			writeJSON(w, 401, map[string]string{"error": "OIDC callback failed"})
			return
		}
		provider, _ := s.OIDC.provider(key)
		tokens, err := s.AuthService.LoginOIDC(key, claims.Subject, r.URL.Query().Get("username"), r.URL.Query().Get("email"), provider.AutoProvision)
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
