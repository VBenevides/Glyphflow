package api

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type OIDCProvider struct {
	Key, Issuer, ClientID, Callback string
	Enabled, AutoProvision          bool
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
	if err := platform.ValidateOIDCCallback(provider.Callback, provider.Callback); err != nil {
		return err
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
	s.mu.RLock()
	p, ok := s.providers[key]
	s.mu.RUnlock()
	if !ok || !p.Enabled {
		return "", errors.New("OIDC provider is unavailable")
	}
	state, nonce, err := s.states.Create(key, "login", now, 10*time.Minute)
	if err != nil {
		return "", err
	}
	u, _ := url.Parse(p.Issuer)
	q := u.Query()
	q.Set("state", state)
	q.Set("nonce", nonce)
	q.Set("redirect_uri", redirect)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (s *OIDCService) Complete(key, state, nonce string, now time.Time) error {
	return s.states.Consume(state, nonce, key, "login", now)
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
}
