package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type AuthUser struct {
	ID, Username, Email, DisplayName string
	Enabled                          bool
}
type AuthTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	SessionID    string `json:"session_id"`
}

type AuthService struct {
	mu                                   sync.RWMutex
	hasher                               platform.PasswordHasher
	users                                map[string]AuthUser
	byEmail                              map[string]string
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
	adminGuard                           *platform.AdministratorGuard
	systemAdminEmails                    map[string]bool
}

func NewAuthService(accessSecret string, passwordEnabled, registrationEnabled bool, pepper []byte) (*AuthService, error) {
	sessions, err := NewSessionManager(accessSecret)
	if err != nil {
		return nil, err
	}
	return &AuthService{hasher: platform.DefaultPasswordHasher(pepper), users: map[string]AuthUser{}, byEmail: map[string]string{}, passwords: map[string]string{}, oidcIdentities: map[string]string{}, roles: map[string]map[string]bool{}, rolePermissions: map[string]map[string]bool{}, passwordEnabled: passwordEnabled, registrationEnabled: registrationEnabled, sessions: sessions, refresh: platform.NewRefreshSessionManager(), accessLifetime: 15 * time.Minute, refreshLifetime: 30 * time.Hour * 24, adminGuard: platform.NewAdministratorGuard(), systemAdminEmails: map[string]bool{}}, nil
}

func (s *AuthService) SetSystemAdminEmails(emails []string) error {
	configured := map[string]bool{}
	for _, value := range emails {
		email, err := platform.NormalizeEmail(value)
		if err != nil {
			return err
		}
		configured[email] = true
	}
	s.mu.Lock()
	s.systemAdminEmails = configured
	var administrators []string
	for id, user := range s.users {
		if !configured[user.Email] {
			continue
		}
		if s.roles[id] == nil {
			s.roles[id] = map[string]bool{}
		}
		s.roles[id]["admin"] = true
		administrators = append(administrators, id)
	}
	s.mu.Unlock()
	for _, id := range administrators {
		s.adminGuard.Add(id)
	}
	return nil
}

func (s *AuthService) SetDefaultRole(role string) {
	s.mu.Lock()
	s.defaultRole = strings.TrimSpace(role)
	s.mu.Unlock()
}

func (s *AuthService) AuthSettings() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{"passwordLoginEnabled": s.passwordEnabled, "registration": s.registrationEnabled, "defaultRole": s.defaultRole}
}

func (s *AuthService) PasswordLoginEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.passwordEnabled
}

func (s *AuthService) RegistrationEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registrationEnabled
}

func (s *AuthService) SessionManager() *SessionManager { return s.sessions }

func (s *AuthService) UpdateAuthSettings(passwordEnabled, registrationEnabled bool, defaultRole string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if defaultRole != "" {
		if _, ok := s.rolePermissions[defaultRole]; !ok {
			return errors.New("default role not found")
		}
		s.defaultRole = defaultRole
	}
	s.passwordEnabled = passwordEnabled
	s.registrationEnabled = registrationEnabled
	return nil
}

func (s *AuthService) EnsureBootstrap(username, password, provider, subject string) (AuthUser, error) {
	key, err := platform.NormalizeEmail(username)
	if err != nil || password == "" {
		return AuthUser{}, errors.New("bootstrap email and password are required")
	}
	s.mu.RLock()
	existingID := s.byEmail[key]
	s.mu.RUnlock()
	if existingID != "" {
		if err := s.Grant(existingID, "admin"); err != nil {
			return AuthUser{}, err
		}
		user, _ := s.User(existingID)
		return user, nil
	}
	user, err := s.register(key, password, false)
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
	if _, ok := s.users[userID]; !ok {
		s.mu.Unlock()
		return errors.New("user not found")
	}
	if _, ok := s.rolePermissions[role]; !ok {
		s.mu.Unlock()
		return errors.New("role not found")
	}
	if s.roles[userID] == nil {
		s.roles[userID] = map[string]bool{}
	}
	s.roles[userID][role] = true
	s.mu.Unlock()
	if role == "admin" {
		s.adminGuard.Add(userID)
	}
	return nil
}

func (s *AuthService) Register(email, password string) (AuthUser, error) {
	return s.register(email, password, true)
}

func (s *AuthService) register(email, password string, requireRegistration bool) (AuthUser, error) {
	key, err := platform.NormalizeEmail(email)
	if err != nil {
		return AuthUser{}, err
	}
	if requireRegistration && (!s.passwordEnabled || !s.registrationEnabled) {
		return AuthUser{}, errors.New("registration is disabled")
	}
	if err := platform.ValidatePassword(password); err != nil {
		return AuthUser{}, err
	}
	s.mu.RLock()
	role := s.defaultRole
	_, exists := s.byEmail[key]
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
	user := AuthUser{ID: id, Username: key, Email: key, Enabled: true}
	s.mu.Lock()
	if _, exists = s.byEmail[key]; exists {
		s.mu.Unlock()
		return AuthUser{}, errors.New("registration failed")
	}
	s.users[id], s.byEmail[key], s.passwords[id] = user, id, hash
	s.roles[id] = map[string]bool{role: true}
	systemAdmin := s.systemAdminEmails[key]
	if systemAdmin {
		s.roles[id]["admin"] = true
	}
	audit := s.audit
	s.mu.Unlock()
	if systemAdmin {
		s.adminGuard.Add(id)
	}
	if audit != nil {
		audit("system", "user.register", id)
	}
	return user, nil
}

func (s *AuthService) Login(email, password string) (AuthTokens, error) {
	key, err := platform.NormalizeEmail(email)
	if err != nil {
		return AuthTokens{}, errors.New("invalid credentials")
	}
	s.mu.RLock()
	userID := s.byEmail[key]
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
	email, emailErr := platform.NormalizeEmail(email)
	if provider == "" || subject == "" || emailErr != nil {
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
		if _, exists := s.byEmail[email]; exists {
			s.mu.Unlock()
			return AuthTokens{}, errors.New("OIDC email is already registered")
		}
		userID, _ = randomID()
		s.users[userID] = AuthUser{ID: userID, Username: email, Email: email, Enabled: true}
		s.byEmail[email], s.roles[userID] = userID, map[string]bool{s.defaultRole: true}
		if s.systemAdminEmails[email] {
			s.roles[userID]["admin"] = true
		}
		s.oidcIdentities[key] = userID
	}
	user, exists := s.users[userID]
	systemAdmin := exists && s.systemAdminEmails[user.Email]
	audit := s.audit
	s.mu.Unlock()
	if !exists || !user.Enabled {
		return AuthTokens{}, errors.New("OIDC identity is not linked")
	}
	if systemAdmin {
		s.adminGuard.Add(user.ID)
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

func (s *AuthService) UnlinkOIDC(userID, identityID string) error {
	keyBytes, err := base64.RawURLEncoding.DecodeString(identityID)
	if err != nil {
		return errors.New("identity not found")
	}
	key := string(keyBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.oidcIdentities[key] != userID {
		return errors.New("identity not found")
	}
	delete(s.oidcIdentities, key)
	return nil
}

func (s *AuthService) UpdateProfile(userID, displayName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[userID]
	if !ok {
		return errors.New("user not found")
	}
	user.DisplayName = strings.TrimSpace(displayName)
	s.users[userID] = user
	return nil
}

func (s *AuthService) ChangePassword(userID, currentPassword, newPassword string) error {
	if err := platform.ValidatePassword(newPassword); err != nil {
		return err
	}
	s.mu.RLock()
	user, exists := s.users[userID]
	hash := s.passwords[userID]
	s.mu.RUnlock()
	if !exists || !user.Enabled || hash == "" {
		return errors.New("password change unavailable")
	}
	valid, err := s.hasher.Verify(hash, currentPassword)
	if err != nil || !valid {
		return errors.New("current password is invalid")
	}
	updated, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.passwords[userID] = updated
	s.mu.Unlock()
	return nil
}

func (s *AuthService) Identities(userID string) []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	identities := []map[string]any{}
	for key, owner := range s.oidcIdentities {
		if owner != userID {
			continue
		}
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		identities = append(identities, map[string]any{"id": base64.RawURLEncoding.EncodeToString([]byte(key)), "provider": parts[0], "subject": parts[1]})
	}
	sort.Slice(identities, func(i, j int) bool { return identities[i]["id"].(string) < identities[j]["id"].(string) })
	return identities
}

func (s *AuthService) Users() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]map[string]any, 0, len(s.users))
	for id, user := range s.users {
		userRoles := s.rolesForUserLocked(id)
		roles := make([]string, 0, len(userRoles))
		for role := range userRoles {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		methods := []string{}
		if s.passwords[id] != "" {
			methods = append(methods, "password")
		}
		for key, owner := range s.oidcIdentities {
			if owner == id {
				methods = append(methods, strings.SplitN(key, "\x00", 2)[0])
			}
		}
		sort.Strings(methods)
		users = append(users, map[string]any{"id": user.ID, "username": user.Username, "email": user.Email, "displayName": user.DisplayName, "enabled": user.Enabled, "systemAdmin": s.systemAdminEmails[user.Email], "status": map[bool]string{true: "active", false: "disabled"}[user.Enabled], "roles": roles, "loginMethods": methods, "sessions": s.sessions.List(id)})
	}
	sort.Slice(users, func(i, j int) bool { return users[i]["username"].(string) < users[j]["username"].(string) })
	return users
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
	s.mu.RLock()
	user, ok := s.users[userID]
	isAdmin := s.roles[userID]["admin"]
	systemAdmin := s.systemAdminEmails[user.Email]
	s.mu.RUnlock()
	if !ok {
		return errors.New("user not found")
	}
	if systemAdmin {
		return platform.ErrSystemAdministrator
	}
	disable := func() error {
		s.mu.Lock()
		user.Enabled = false
		s.users[userID] = user
		s.mu.Unlock()
		s.LogoutAll(userID)
		return nil
	}
	if isAdmin {
		return s.adminGuard.Remove(userID, disable)
	}
	return disable()
}

func (s *AuthService) Revoke(userID, role string) error {
	s.mu.RLock()
	user, ok := s.users[userID]
	assigned := s.roles[userID][role]
	systemAdmin := ok && s.systemAdminEmails[user.Email]
	s.mu.RUnlock()
	if !ok {
		return errors.New("user not found")
	}
	if role == "admin" && systemAdmin {
		return platform.ErrSystemAdministrator
	}
	if !assigned {
		return errors.New("role not assigned")
	}
	if role == "admin" {
		return s.adminGuard.Remove(userID, func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			user, ok := s.users[userID]
			if !ok {
				return errors.New("user not found")
			}
			if s.systemAdminEmails[user.Email] {
				return platform.ErrSystemAdministrator
			}
			if !s.roles[userID][role] {
				return errors.New("role not assigned")
			}
			delete(s.roles[userID], role)
			return nil
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.roles[userID], role)
	return nil
}
func (s *AuthService) Permissions(claims Claims) map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := map[string]bool{}
	for role := range s.rolesForUserLocked(claims.UserID) {
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
	systemAdmin := s.systemAdminEmails[user.Email]
	roles := []string{}
	roleSources := []string{}
	permissions := map[string]bool{}
	for role := range s.rolesForUserLocked(claims.UserID) {
		roles = append(roles, role)
		source := "assigned"
		if role == "admin" && systemAdmin {
			source = "system-admin"
		} else if role == "admin" || role == "user" {
			source = "system"
		}
		roleSources = append(roleSources, role+":"+source)
		for permission := range s.rolePermissions[role] {
			permissions[permission] = true
		}
	}
	s.mu.RUnlock()
	sort.Strings(roles)
	sort.Strings(roleSources)
	permissionKeys := make([]string, 0, len(permissions))
	for permission := range permissions {
		permissionKeys = append(permissionKeys, permission)
	}
	sort.Strings(permissionKeys)
	sessions := s.sessions.List(claims.UserID)
	for index := range sessions {
		sessions[index].Current = sessions[index].ID == claims.SessionID
	}
	methods := []string{}
	s.mu.RLock()
	if s.passwords[claims.UserID] != "" {
		methods = append(methods, "password")
	}
	for key, owner := range s.oidcIdentities {
		if owner == claims.UserID {
			methods = append(methods, strings.SplitN(key, "\x00", 2)[0])
		}
	}
	s.mu.RUnlock()
	sort.Strings(methods)
	return map[string]any{"id": user.ID, "username": user.Username, "email": user.Email, "displayName": user.DisplayName, "enabled": user.Enabled, "systemAdmin": systemAdmin, "status": map[bool]string{true: "active", false: "disabled"}[user.Enabled], "roles": roles, "roleSources": roleSources, "permissions": permissionKeys, "loginMethods": methods, "sessions": sessions, "identities": s.Identities(claims.UserID)}
}

func (s *AuthService) UserProfile(userID string) (map[string]any, bool) {
	s.mu.RLock()
	_, ok := s.users[userID]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return s.Profile(Claims{UserID: userID}), true
}

func (s *AuthService) rolesForUserLocked(userID string) map[string]bool {
	roles := map[string]bool{}
	for role := range s.roles[userID] {
		roles[role] = true
	}
	if user := s.users[userID]; s.systemAdminEmails[user.Email] {
		roles["admin"] = true
	}
	return roles
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
