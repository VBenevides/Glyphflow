package api

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type AuthUser struct {
	ID, Username, Email string
	Enabled             bool
}
type AuthTokens struct{ AccessToken, RefreshToken, SessionID string }

type AuthService struct {
	mu                                   sync.RWMutex
	hasher                               platform.PasswordHasher
	users                                map[string]AuthUser
	byUsername                           map[string]string
	passwords                            map[string]string
	oidcIdentities                       map[string]string
	roles                                map[string]map[string]bool
	rolePermissions                      map[string]map[string]bool
	passwordEnabled, registrationEnabled bool
	defaultRole                          string
	sessions                             *SessionManager
	refresh                              *platform.RefreshSessionManager
	accessLifetime, refreshLifetime      time.Duration
	audit                                func(string, string, string)
}

func NewAuthService(accessSecret string, passwordEnabled, registrationEnabled bool, pepper []byte) (*AuthService, error) {
	sessions, err := NewSessionManager(accessSecret)
	if err != nil {
		return nil, err
	}
	return &AuthService{hasher: platform.DefaultPasswordHasher(pepper), users: map[string]AuthUser{}, byUsername: map[string]string{}, passwords: map[string]string{}, oidcIdentities: map[string]string{}, roles: map[string]map[string]bool{}, rolePermissions: map[string]map[string]bool{}, passwordEnabled: passwordEnabled, registrationEnabled: registrationEnabled, sessions: sessions, refresh: platform.NewRefreshSessionManager(), accessLifetime: 15 * time.Minute, refreshLifetime: 30 * 24 * time.Hour}, nil
}

func (s *AuthService) SetDefaultRole(role string) {
	s.mu.Lock()
	s.defaultRole = strings.TrimSpace(role)
	s.mu.Unlock()
}

func (s *AuthService) EnsureBootstrap(username, password, provider, subject string) (AuthUser, error) {
	key := platform.NormalizeIdentityKey(username)
	if key == "" {
		return AuthUser{}, errors.New("bootstrap username is required")
	}
	s.mu.RLock()
	existingID := s.byUsername[key]
	s.mu.RUnlock()
	if existingID != "" {
		if err := s.Grant(existingID, "admin"); err != nil {
			return AuthUser{}, err
		}
		user, _ := s.User(existingID)
		return user, nil
	}
	var user AuthUser
	var err error
	if password != "" {
		user, err = s.Register(key, password)
	} else if provider != "" && subject != "" {
		s.mu.Lock()
		if s.defaultRole == "" {
			s.mu.Unlock()
			return AuthUser{}, errors.New("default role is not configured")
		}
		id, idErr := randomID()
		if idErr != nil {
			s.mu.Unlock()
			return AuthUser{}, idErr
		}
		user = AuthUser{ID: id, Username: key, Enabled: true}
		s.users[id] = user
		s.byUsername[key] = id
		s.roles[id] = map[string]bool{s.defaultRole: true}
		s.oidcIdentities[platform.NormalizeIdentityKey(provider)+"\x00"+subject] = id
		s.mu.Unlock()
	} else {
		return AuthUser{}, errors.New("bootstrap password or OIDC identity is required")
	}
	if err != nil {
		return AuthUser{}, err
	}
	if err := s.Grant(user.ID, "admin"); err != nil {
		return AuthUser{}, err
	}
	return user, nil
}
func (s *AuthService) SetAudit(fn func(string, string, string)) {
	s.mu.Lock()
	s.audit = fn
	s.mu.Unlock()
}
func (s *AuthService) AddRole(role string, permissions ...string) error {
	if role == "" {
		return errors.New("role is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rolePermissions[role]; ok {
		return errors.New("role already exists")
	}
	s.rolePermissions[role] = map[string]bool{}
	for _, permission := range permissions {
		s.rolePermissions[role][permission] = true
	}
	return nil
}
func (s *AuthService) Grant(userID, role string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return errors.New("user not found")
	}
	if _, ok := s.rolePermissions[role]; !ok {
		return errors.New("role not found")
	}
	if s.roles[userID] == nil {
		s.roles[userID] = map[string]bool{}
	}
	s.roles[userID][role] = true
	return nil
}

func (s *AuthService) Register(username, password string) (AuthUser, error) {
	key := platform.NormalizeIdentityKey(username)
	if !s.passwordEnabled || !s.registrationEnabled {
		return AuthUser{}, errors.New("registration is disabled")
	}
	if key == "" {
		return AuthUser{}, errors.New("username and password are required")
	}
	if err := platform.ValidatePassword(password); err != nil {
		return AuthUser{}, err
	}
	s.mu.RLock()
	role := s.defaultRole
	_, exists := s.byUsername[key]
	s.mu.RUnlock()
	if role == "" {
		return AuthUser{}, errors.New("default role is not configured")
	}
	if exists {
		return AuthUser{}, errors.New("registration failed")
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return AuthUser{}, err
	}
	id, err := randomID()
	if err != nil {
		return AuthUser{}, err
	}
	user := AuthUser{ID: id, Username: key, Enabled: true}
	s.mu.Lock()
	if _, exists = s.byUsername[key]; exists {
		s.mu.Unlock()
		return AuthUser{}, errors.New("registration failed")
	}
	s.users[id], s.byUsername[key], s.passwords[id] = user, id, hash
	s.roles[id] = map[string]bool{role: true}
	audit := s.audit
	s.mu.Unlock()
	if audit != nil {
		audit("system", "user.register", id)
	}
	return user, nil
}

func (s *AuthService) Login(username, password string) (AuthTokens, error) {
	key := platform.NormalizeIdentityKey(username)
	s.mu.RLock()
	userID := s.byUsername[key]
	hash := s.passwords[userID]
	user, ok := s.users[userID]
	enabled := s.passwordEnabled
	audit := s.audit
	s.mu.RUnlock()
	valid := false
	if enabled && ok && user.Enabled && hash != "" {
		valid, _ = s.hasher.Verify(hash, password)
	}
	if !valid {
		if audit != nil {
			audit("system", "auth.login.failed", key)
		}
		return AuthTokens{}, errors.New("invalid credentials")
	}
	tokens, err := s.issueTokens(user.ID)
	if err != nil {
		return AuthTokens{}, err
	}
	if audit != nil {
		audit(user.ID, "auth.login", tokens.SessionID)
	}
	return tokens, nil
}

func (s *AuthService) LoginOIDC(provider, subject, username, email string, autoProvision bool) (AuthTokens, error) {
	provider, subject = platform.NormalizeIdentityKey(provider), strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return AuthTokens{}, errors.New("OIDC identity is incomplete")
	}
	key := provider + "\x00" + subject
	s.mu.Lock()
	userID := s.oidcIdentities[key]
	if userID == "" && autoProvision {
		if s.defaultRole == "" {
			s.mu.Unlock()
			return AuthTokens{}, errors.New("default role is not configured")
		}
		base := platform.NormalizeIdentityKey(username)
		if base == "" {
			base = provider + "-" + subject
		}
		if _, exists := s.byUsername[base]; exists {
			base = base + "-" + subject[:minInt(8, len(subject))]
		}
		userID, _ = randomID()
		s.users[userID] = AuthUser{ID: userID, Username: base, Email: email, Enabled: true}
		s.byUsername[base], s.roles[userID] = userID, map[string]bool{s.defaultRole: true}
		s.oidcIdentities[key] = userID
	}
	user, exists := s.users[userID]
	audit := s.audit
	s.mu.Unlock()
	if !exists || !user.Enabled {
		return AuthTokens{}, errors.New("OIDC identity is not linked")
	}
	tokens, err := s.issueTokens(user.ID)
	if err != nil {
		return AuthTokens{}, err
	}
	if audit != nil {
		audit(user.ID, "auth.oidc.login", provider)
	}
	return tokens, nil
}

func (s *AuthService) LinkOIDC(userID, provider, subject string) error {
	provider, subject = platform.NormalizeIdentityKey(provider), strings.TrimSpace(subject)
	if userID == "" || provider == "" || subject == "" {
		return errors.New("OIDC link is incomplete")
	}
	key := provider + "\x00" + subject
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[userID]; !ok {
		return errors.New("user not found")
	}
	if existing := s.oidcIdentities[key]; existing != "" && existing != userID {
		return errors.New("OIDC identity already linked")
	}
	s.oidcIdentities[key] = userID
	return nil
}

func (s *AuthService) issueTokens(userID string) (AuthTokens, error) {
	sessionID, refreshToken, err := s.refresh.Issue(userID, s.refreshLifetime)
	if err != nil {
		return AuthTokens{}, err
	}
	accessToken, _, err := s.sessions.IssueForSession(userID, sessionID, s.accessLifetime)
	if err != nil {
		return AuthTokens{}, err
	}
	return AuthTokens{AccessToken: accessToken, RefreshToken: refreshToken, SessionID: sessionID}, nil
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *AuthService) Refresh(sessionID, refreshToken string) (AuthTokens, error) {
	userID, ok := s.refresh.UserID(sessionID)
	if !ok {
		return AuthTokens{}, errors.New("refresh token is invalid")
	}
	newID, newToken, err := s.refresh.Rotate(sessionID, refreshToken, s.refreshLifetime)
	if err != nil {
		if s.audit != nil {
			s.audit(userID, "auth.refresh.replay", sessionID)
		}
		return AuthTokens{}, err
	}
	access, _, err := s.sessions.IssueForSession(userID, newID, s.accessLifetime)
	if err != nil {
		return AuthTokens{}, err
	}
	s.sessions.Revoke(sessionID)
	return AuthTokens{AccessToken: access, RefreshToken: newToken, SessionID: newID}, nil
}

func (s *AuthService) Logout(sessionID string) {
	s.refresh.Revoke(sessionID)
	s.sessions.Revoke(sessionID)
}
func (s *AuthService) LogoutAll(userID string) {
	s.refresh.RevokeUser(userID)
	s.sessions.RevokeUser(userID)
}
func (s *AuthService) DisableUser(userID string) error {
	s.mu.Lock()
	user, ok := s.users[userID]
	if ok {
		user.Enabled = false
		s.users[userID] = user
	}
	s.mu.Unlock()
	if !ok {
		return errors.New("user not found")
	}
	s.LogoutAll(userID)
	return nil
}
func (s *AuthService) Permissions(claims Claims) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]bool{}
	for role := range s.roles[claims.UserID] {
		for permission := range s.rolePermissions[role] {
			out[permission] = true
		}
	}
	return out
}
func (s *AuthService) Authenticator() Authenticator {
	return func(r *http.Request) (Claims, bool) {
		claims, ok := s.sessions.Authenticator()(r)
		if !ok {
			return Claims{}, false
		}
		s.mu.RLock()
		user, exists := s.users[claims.UserID]
		s.mu.RUnlock()
		if !exists || !user.Enabled {
			return Claims{}, false
		}
		return claims, true
	}
}
func (s *AuthService) User(userID string) (AuthUser, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	user, ok := s.users[userID]
	return user, ok
}

func (s *AuthService) Profile(claims Claims) map[string]any {
	s.mu.RLock()
	user := s.users[claims.UserID]
	roles := []string{}
	permissions := map[string]bool{}
	for role := range s.roles[claims.UserID] {
		roles = append(roles, role)
		for permission := range s.rolePermissions[role] {
			permissions[permission] = true
		}
	}
	s.mu.RUnlock()
	return map[string]any{"id": user.ID, "username": user.Username, "email": user.Email, "roles": roles, "permissions": permissions, "sessions": s.sessions.List(claims.UserID)}
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
