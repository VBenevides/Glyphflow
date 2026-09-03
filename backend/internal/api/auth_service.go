package api

import (
	"context"
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
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

var ErrPendingUser = errors.New("account pending administrator approval")

const defaultAdminDisplayName = "Default Admin"

type AuthUser struct {
	ID, Username, Email, DisplayName string
	Status                           string
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
	dummyPasswordHash                    string
	users                                store.UserRepository
	oidcIdentities                       map[string]string
	ssoRepository                        store.SSORepository
	roles                                store.RoleRepository
	config                               *store.ConfigStore
	passwordEnabled, registrationEnabled bool
	userApprovalRequired                 bool
	defaultRoleID                        string
	lockdownScheduler                    bool
	sessions                             *SessionManager
	sessionRepository                    store.SessionRepository
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
	hasher := platform.DefaultPasswordHasher(pepper)
	dummyPasswordHash, err := hasher.Hash("glyphflow-invalid-login-dummy")
	if err != nil {
		return nil, err
	}
	return &AuthService{hasher: hasher, dummyPasswordHash: dummyPasswordHash, users: newMemoryUserRepository(), oidcIdentities: map[string]string{}, roles: newMemoryRoleRepository(), passwordEnabled: passwordEnabled, registrationEnabled: registrationEnabled, sessions: sessions, refresh: platform.NewRefreshSessionManager(), accessLifetime: 15 * time.Minute, refreshLifetime: 30 * time.Hour * 24, adminGuard: platform.NewAdministratorGuard(), systemAdminEmails: map[string]bool{}}, nil
}

func (s *AuthService) SetUserRepository(repository store.UserRepository) {
	if repository == nil {
		return
	}
	s.mu.Lock()
	s.users = repository
	s.mu.Unlock()
}

func (s *AuthService) SetRoleRepository(repository store.RoleRepository) {
	if repository == nil {
		return
	}
	s.mu.Lock()
	s.roles = repository
	s.mu.Unlock()
}

func (s *AuthService) SetConfigStore(config *store.ConfigStore) {
	s.mu.Lock()
	s.config = config
	s.mu.Unlock()
}

func (s *AuthService) SetSessionRepository(repository store.SessionRepository) {
	if repository == nil {
		return
	}
	s.mu.Lock()
	s.sessionRepository = repository
	s.mu.Unlock()
	s.sessions.SetRepository(repository)
}

func (s *AuthService) SetSSORepository(repository store.SSORepository) {
	if repository == nil {
		return
	}
	s.mu.Lock()
	s.ssoRepository = repository
	s.mu.Unlock()
}

func toAuthUser(user store.UserRecord) AuthUser {
	status := user.Status
	if status == "" {
		status = store.StatusDisabled
		if user.Enabled {
			status = store.StatusActive
		}
	}
	return AuthUser{ID: user.ID, Username: user.Username, Email: user.Email, DisplayName: store.NormalizeDisplayName(user.Email, user.DisplayName), Status: status, Enabled: status == store.StatusActive}
}

func (s *AuthService) userByID(id string) (AuthUser, bool, error) {
	user, ok, err := s.users.FindByID(context.Background(), id)
	return toAuthUser(user), ok, err
}

func (s *AuthService) userByEmail(email string) (AuthUser, bool, error) {
	user, ok, err := s.users.FindByEmail(context.Background(), email)
	return toAuthUser(user), ok, err
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
	users, err := s.users.List(context.Background(), "")
	if err != nil {
		return err
	}
	var systemAdminUsers, activeAdministrators []string
	for _, record := range users {
		if configured[record.Email] {
			systemAdminUsers = append(systemAdminUsers, record.ID)
		}
		roles, _, roleErr := s.roles.UserRoles(context.Background(), record.ID)
		if roleErr != nil {
			return roleErr
		}
		for _, role := range roles {
			if role.Name == "admin" && record.Enabled {
				activeAdministrators = append(activeAdministrators, record.ID)
				break
			}
		}
	}
	adminRole, found, err := s.roles.FindByName(context.Background(), "admin")
	if err != nil {
		return err
	}
	if !found && len(systemAdminUsers) > 0 {
		return errors.New("admin role is not configured")
	}
	if found {
		if err := s.roles.ReplaceSourceAssignments(context.Background(), adminRole.ID, systemAdminSource, systemAdminUsers); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.systemAdminEmails = configured
	s.mu.Unlock()
	s.adminGuard.Set(activeAdministrators...)
	return nil
}

func hasSystemAdminAssignment(roles []store.RoleRecord, assignments []store.RoleAssignmentRecord) bool {
	adminRoles := map[string]bool{}
	for _, role := range roles {
		if role.Name == "admin" {
			adminRoles[role.ID] = true
		}
	}
	for _, assignment := range assignments {
		if assignment.SourceType == systemAdminSource && adminRoles[assignment.RoleID] {
			return true
		}
	}
	return false
}

func (s *AuthService) SetDefaultRole(role string) {
	if definition, ok, _ := s.roles.FindByID(context.Background(), strings.TrimSpace(role)); ok {
		role = definition.ID
	} else if definition, ok, _ := s.roles.FindByName(context.Background(), role); ok {
		role = definition.ID
	}
	s.mu.Lock()
	s.defaultRoleID = strings.TrimSpace(role)
	s.mu.Unlock()
}

func (s *AuthService) SetDefaultRoleID(roleID string) error {
	definition, ok, err := s.roles.FindByID(context.Background(), strings.TrimSpace(roleID))
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("default role not found")
	}
	s.mu.Lock()
	s.defaultRoleID = definition.ID
	s.mu.Unlock()
	return nil
}

func (s *AuthService) AuthSettings() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{"passwordLoginEnabled": s.passwordEnabled, "registration": s.registrationEnabled, "requireUserApproval": s.userApprovalRequired, "defaultRoleId": s.defaultRoleID, "lockdownScheduler": s.lockdownScheduler}
}

func (s *AuthService) LockdownScheduler() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lockdownScheduler
}

func (s *AuthService) SetLockdownScheduler(enabled bool) {
	s.mu.Lock()
	s.lockdownScheduler = enabled
	s.mu.Unlock()
}

func (s *AuthService) UpdateLockdownScheduler(enabled bool) error {
	s.mu.RLock()
	config := s.config
	s.mu.RUnlock()
	if config != nil {
		if err := config.Set(context.Background(), "LOCKDOWN_SCHEDULER", enabled); err != nil {
			return err
		}
	}
	s.SetLockdownScheduler(enabled)
	return nil
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

func (s *AuthService) UserApprovalRequired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userApprovalRequired
}

func (s *AuthService) SetUserApprovalRequired(required bool) {
	s.mu.Lock()
	s.userApprovalRequired = required
	s.mu.Unlock()
}

func (s *AuthService) SessionManager() *SessionManager { return s.sessions }

func (s *AuthService) UpdateAuthSettings(passwordEnabled, registrationEnabled bool, defaultRole string, userApproval ...bool) error {
	defaultRoleID := defaultRole
	if defaultRole != "" {
		definition, ok, err := s.roles.FindByID(context.Background(), defaultRole)
		if err != nil {
			return err
		}
		if !ok {
			definition, ok, err = s.roles.FindByName(context.Background(), defaultRole)
			if err != nil {
				return err
			}
		}
		if !ok {
			return errors.New("default role not found")
		}
		defaultRoleID = definition.ID
	}
	s.mu.RLock()
	if defaultRoleID == "" {
		defaultRoleID = s.defaultRoleID
	}
	config := s.config
	approvalRequired := s.userApprovalRequired
	if len(userApproval) > 0 {
		approvalRequired = userApproval[0]
	}
	s.mu.RUnlock()
	if config != nil {
		if err := config.SetAuthenticationSettings(context.Background(), passwordEnabled, registrationEnabled, defaultRoleID, approvalRequired); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if defaultRoleID != "" {
		s.defaultRoleID = defaultRoleID
	}
	s.passwordEnabled = passwordEnabled
	s.registrationEnabled = registrationEnabled
	s.userApprovalRequired = approvalRequired
	return nil
}

func (s *AuthService) EnsureBootstrap(username, password, provider, subject string) (AuthUser, error) {
	key, err := platform.NormalizeEmail(username)
	if err != nil || password == "" {
		return AuthUser{}, errors.New("bootstrap email and password are required")
	}
	stored, found, findErr := s.users.FindByEmail(context.Background(), key)
	if findErr != nil {
		return AuthUser{}, findErr
	}
	if found {
		existing := toAuthUser(stored)
		if strings.TrimSpace(stored.DisplayName) == "" {
			if err := s.users.UpdateDisplayName(context.Background(), stored.ID, defaultAdminDisplayName); err != nil {
				return AuthUser{}, err
			}
			existing.DisplayName = defaultAdminDisplayName
		}
		if existing.Status != store.StatusActive {
			if err := s.users.SetStatus(context.Background(), existing.ID, store.StatusActive); err != nil {
				return AuthUser{}, err
			}
			existing.Status = store.StatusActive
			existing.Enabled = true
		}
		if err := s.Grant(existing.ID, "admin"); err != nil {
			return AuthUser{}, err
		}
		return existing, nil
	}
	user, err := s.registerWithStatus(key, password, false, store.StatusActive)
	if err != nil {
		return AuthUser{}, err
	}
	if err := s.Grant(user.ID, "admin"); err != nil {
		return AuthUser{}, err
	}
	user.DisplayName = defaultAdminDisplayName
	if err := s.users.UpdateDisplayName(context.Background(), user.ID, defaultAdminDisplayName); err != nil {
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
	if err := validatePermissions(permissions); err != nil {
		return err
	}
	roleID := systemRoleID(role)
	if err := s.roles.Ensure(context.Background(), roleID, platform.NormalizeIdentityKey(role), "", role == "admin" || role == "user" || role == "operator", permissions); err != nil {
		return err
	}
	s.mu.Lock()
	if s.defaultRoleID == role || s.defaultRoleID == platform.NormalizeIdentityKey(role) {
		s.defaultRoleID = roleID
	}
	s.mu.Unlock()
	return nil
}
func (s *AuthService) Grant(userID, role string) error {
	user, ok, err := s.userByID(userID)
	if err != nil {
		return err
	} else if !ok {
		return errors.New(errorUserNotFound)
	}
	definition, ok, err := s.roles.FindByName(context.Background(), role)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("role not found")
	}
	if err := s.roles.Assign(context.Background(), userID, definition.ID, "manual", "auth-service"); err != nil {
		return err
	}
	if role == "admin" && user.Status == store.StatusActive {
		s.adminGuard.Add(userID)
	}
	return nil
}

func (s *AuthService) Register(email, password string) (AuthUser, error) {
	return s.register(email, password, true)
}

func (s *AuthService) register(email, password string, requireRegistration bool) (AuthUser, error) {
	return s.registerWithStatus(email, password, requireRegistration, "")
}

func (s *AuthService) registerWithStatus(email, password string, requireRegistration bool, requestedStatus string) (AuthUser, error) {
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
	roleID := s.defaultRoleID
	systemAdmin := s.systemAdminEmails[key]
	approvalRequired := s.userApprovalRequired
	s.mu.RUnlock()
	if roleID == "" {
		return AuthUser{}, errors.New("default role is not configured")
	}
	if _, exists, err := s.userByEmail(key); err != nil {
		return AuthUser{}, err
	} else if exists {
		return AuthUser{}, errors.New("registration failed")
	}
	roleDefinition, adminRoleID, err := s.registrationRoles(roleID, systemAdmin)
	if err != nil {
		return AuthUser{}, err
	}
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return AuthUser{}, err
	}
	id, err := randomID()
	if err != nil {
		return AuthUser{}, err
	}
	status := requestedStatus
	if status == "" {
		status = store.StatusActive
		if approvalRequired {
			status = store.StatusPending
		}
	}
	user := AuthUser{ID: id, Username: key, Email: key, DisplayName: store.DefaultUserDisplayName(key), Status: status, Enabled: status == store.StatusActive}
	userRecord := store.UserRecord{ID: user.ID, Username: user.Username, Email: user.Email, DisplayName: user.DisplayName, Status: user.Status, Enabled: user.Enabled}
	if err := s.provisionRegistration(userRecord, hash, roleDefinition.ID, adminRoleID); err != nil {
		return AuthUser{}, err
	}
	s.mu.Lock()
	audit := s.audit
	s.mu.Unlock()
	if systemAdmin && user.Enabled {
		s.adminGuard.Add(id)
	}
	if audit != nil {
		audit("system", "user.register", id)
	}
	return user, nil
}

func (s *AuthService) registrationRoles(roleID string, systemAdmin bool) (store.RoleRecord, string, error) {
	roleDefinition, found, err := s.roles.FindByID(context.Background(), roleID)
	if err != nil {
		return store.RoleRecord{}, "", err
	}
	if !found {
		return store.RoleRecord{}, "", errors.New("default role is not configured")
	}
	if !systemAdmin {
		return roleDefinition, "", nil
	}
	adminRole, found, err := s.roles.FindByName(context.Background(), "admin")
	if err != nil || !found {
		return store.RoleRecord{}, "", errors.New("admin role is not configured")
	}
	return roleDefinition, adminRole.ID, nil
}

func (s *AuthService) provisionRegistration(user store.UserRecord, hash, roleID, adminRoleID string) error {
	if provisioner, ok := s.users.(interface {
		ProvisionLocal(context.Context, store.UserRecord, string, string, string) error
	}); ok {
		if err := provisioner.ProvisionLocal(context.Background(), user, hash, roleID, adminRoleID); err != nil {
			return errors.New("registration failed")
		}
		return nil
	}
	if err := s.users.Create(context.Background(), user, hash); err != nil {
		return errors.New("registration failed")
	}
	if err := s.roles.Assign(context.Background(), user.ID, roleID, "default", roleID); err != nil {
		return err
	}
	if adminRoleID != "" {
		if err := s.roles.Assign(context.Background(), user.ID, adminRoleID, systemAdminSource, user.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *AuthService) Login(email, password string) (AuthTokens, error) {
	key, err := platform.NormalizeEmail(email)
	if err != nil {
		return AuthTokens{}, errors.New("invalid credentials")
	}
	user, ok, findErr := s.userByEmail(key)
	if findErr != nil {
		return AuthTokens{}, errors.New("invalid credentials")
	}
	hash, hasHash, _ := s.users.PasswordHash(context.Background(), user.ID)
	s.mu.RLock()
	enabled := s.passwordEnabled
	audit := s.audit
	s.mu.RUnlock()
	valid := false
	if enabled && ok && user.Status == store.StatusPending && hasHash {
		valid, _ = s.hasher.Verify(hash, password)
		if valid {
			return AuthTokens{}, ErrPendingUser
		}
	} else if enabled && ok && user.Enabled && hasHash {
		valid, _ = s.hasher.Verify(hash, password)
		if valid && s.hasher.NeedsRehash(hash) {
			if upgraded, err := s.hasher.Hash(password); err == nil {
				_ = s.users.SetPasswordHash(context.Background(), user.ID, upgraded)
			}
		}
	} else if enabled {
		_, _ = s.hasher.Verify(s.dummyPasswordHash, password)
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
	return s.loginOIDC(provider, subject, username, email, autoProvision, nil)
}

func (s *AuthService) LoginOIDCWithGroups(provider, subject, username, email string, autoProvision bool, groups []string) (AuthTokens, error) {
	return s.loginOIDC(provider, subject, username, email, autoProvision, groups)
}

func (s *AuthService) loginOIDC(provider, subject, username, email string, autoProvision bool, groups []string) (AuthTokens, error) {
	provider, subject = platform.NormalizeIdentityKey(provider), strings.TrimSpace(subject)
	email, emailErr := platform.NormalizeEmail(email)
	if provider == "" || subject == "" || emailErr != nil {
		return AuthTokens{}, errors.New("OIDC identity is incomplete")
	}
	key := provider + "\x00" + subject
	s.mu.RLock()
	userID := s.oidcIdentities[key]
	ssoRepository := s.ssoRepository
	defaultRoleID := s.defaultRoleID
	systemAdmin := s.systemAdminEmails[email]
	audit := s.audit
	s.mu.RUnlock()
	if ssoRepository != nil {
		identity, found, err := ssoRepository.FindIdentity(context.Background(), provider, subject)
		if err != nil {
			return AuthTokens{}, err
		}
		if found {
			userID = identity.UserID
		}
	}
	if userID == "" && autoProvision {
		var err error
		userID, err = s.provisionOIDCUser(provider, subject, email, defaultRoleID, systemAdmin, ssoRepository, key)
		if err != nil {
			return AuthTokens{}, err
		}
	}
	user, exists, err := s.userByID(userID)
	if err != nil {
		return AuthTokens{}, err
	}
	if !exists {
		return AuthTokens{}, errors.New("OIDC identity is not linked")
	}
	if user.Status == store.StatusPending {
		return AuthTokens{}, ErrPendingUser
	}
	if !user.Enabled {
		return AuthTokens{}, errors.New("OIDC identity is not linked")
	}
	if systemAdmin {
		s.adminGuard.Add(user.ID)
	}
	if err := s.syncOIDCRoleAssignments(user.ID, provider, groups, ssoRepository); err != nil {
		return AuthTokens{}, err
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

func (s *AuthService) provisionOIDCUser(provider, subject, email, defaultRoleID string, systemAdmin bool, ssoRepository store.SSORepository, identityKey string) (string, error) {
	if defaultRoleID == "" {
		return "", errors.New("default role is not configured")
	}
	if _, exists, err := s.userByEmail(email); err != nil {
		return "", err
	} else if exists {
		return "", errors.New("OIDC email is already registered")
	}
	userID, err := randomID()
	if err != nil {
		return "", err
	}
	roleDefinition, found, err := s.roles.FindByID(context.Background(), defaultRoleID)
	if err != nil || !found {
		return "", errors.New("default role is not configured")
	}
	adminRoleID := ""
	if systemAdmin {
		adminRole, found, err := s.roles.FindByName(context.Background(), "admin")
		if err != nil || !found {
			return "", errors.New("admin role is not configured")
		}
		adminRoleID = adminRole.ID
	}
	status := store.StatusActive
	if s.UserApprovalRequired() {
		status = store.StatusPending
	}
	userRecord := store.UserRecord{ID: userID, Username: email, Email: email, DisplayName: store.DefaultUserDisplayName(email), Status: status, Enabled: status == store.StatusActive}
	identity := store.SSOIdentityRecord{ID: "identity-" + userID + "-" + provider, UserID: userID, ProviderID: provider, Subject: subject}
	if provisioner, ok := ssoRepository.(store.OIDCProvisioner); ok {
		if err := provisioner.ProvisionOIDC(context.Background(), userRecord, roleDefinition.ID, adminRoleID, identity); err != nil {
			return "", err
		}
		return userID, nil
	}
	if err := s.users.Create(context.Background(), userRecord, ""); err != nil {
		return "", err
	}
	if err := s.roles.Assign(context.Background(), userID, roleDefinition.ID, "default", roleDefinition.ID); err != nil {
		return "", err
	}
	if adminRoleID != "" {
		if err := s.roles.Assign(context.Background(), userID, adminRoleID, systemAdminSource, userID); err != nil {
			return "", err
		}
	}
	if ssoRepository != nil {
		if err := ssoRepository.CreateIdentity(context.Background(), identity); err != nil {
			return "", err
		}
	} else {
		s.mu.Lock()
		s.oidcIdentities[identityKey] = userID
		s.mu.Unlock()
	}
	return userID, nil
}

func (s *AuthService) syncOIDCRoleAssignments(userID, provider string, groups []string, repository store.SSORepository) error {
	if repository == nil {
		return nil
	}
	mappings, err := repository.ListGroupRoleMappings(context.Background(), provider)
	if err != nil {
		return err
	}
	groupSet := map[string]bool{}
	for _, group := range groups {
		groupSet[group] = true
	}
	assignments := make([]store.RoleAssignmentRecord, 0, len(mappings))
	for _, mapping := range mappings {
		if groupSet[mapping.GroupName] {
			assignments = append(assignments, store.RoleAssignmentRecord{RoleID: mapping.RoleID, SourceKey: mapping.GroupName})
		}
	}
	return s.roles.ReplaceSSOAssignments(context.Background(), userID, provider, assignments)
}

func (s *AuthService) LinkOIDC(userID, provider, subject string) error {
	provider, subject = platform.NormalizeIdentityKey(provider), strings.TrimSpace(subject)
	if userID == "" || provider == "" || subject == "" {
		return errors.New("OIDC link is incomplete")
	}
	if _, ok, err := s.userByID(userID); err != nil {
		return err
	} else if !ok {
		return errors.New(errorUserNotFound)
	}
	key := provider + "\x00" + subject
	s.mu.RLock()
	ssoRepository := s.ssoRepository
	s.mu.RUnlock()
	if ssoRepository != nil {
		identity, found, err := ssoRepository.FindIdentity(context.Background(), provider, subject)
		if err != nil {
			return err
		}
		if found && identity.UserID != userID {
			return errors.New("OIDC identity already linked")
		}
		if found {
			return nil
		}
		return ssoRepository.CreateIdentity(context.Background(), store.SSOIdentityRecord{ID: "identity-" + userID + "-" + provider + "-" + subject, UserID: userID, ProviderID: provider, Subject: subject})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	parts := strings.SplitN(key, "\x00", 2)
	s.mu.RLock()
	ssoRepository := s.ssoRepository
	s.mu.RUnlock()
	if ssoRepository != nil {
		if len(parts) != 2 {
			return errors.New("identity not found")
		}
		return ssoRepository.DeleteIdentity(context.Background(), userID, parts[0], parts[1])
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.oidcIdentities[key] != userID {
		return errors.New("identity not found")
	}
	delete(s.oidcIdentities, key)
	return nil
}

func (s *AuthService) UpdateProfile(userID, displayName string) error {
	if _, ok, err := s.userByID(userID); err != nil {
		return err
	} else if !ok {
		return errors.New(errorUserNotFound)
	}
	return s.users.UpdateDisplayName(context.Background(), userID, strings.TrimSpace(displayName))
}

func (s *AuthService) ChangePassword(userID, currentPassword, newPassword string) error {
	if err := platform.ValidatePassword(newPassword); err != nil {
		return err
	}
	user, exists, findErr := s.userByID(userID)
	if findErr != nil {
		return findErr
	}
	hash, hasHash, hashErr := s.users.PasswordHash(context.Background(), userID)
	if hashErr != nil {
		return hashErr
	}
	s.mu.RLock()
	enabled := s.passwordEnabled
	s.mu.RUnlock()
	if !enabled || !exists || !user.Enabled || !hasHash {
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
	if err := s.users.SetPasswordHash(context.Background(), userID, updated); err != nil {
		return err
	}
	return s.LogoutAll(userID)
}

func (s *AuthService) Identities(userID string) []map[string]any {
	s.mu.RLock()
	ssoRepository := s.ssoRepository
	s.mu.RUnlock()
	if ssoRepository != nil {
		records, err := ssoRepository.ListIdentities(context.Background(), userID)
		if err != nil {
			return []map[string]any{}
		}
		identities := make([]map[string]any, 0, len(records))
		for _, record := range records {
			key := record.ProviderID + "\x00" + record.Subject
			identities = append(identities, map[string]any{"id": base64.RawURLEncoding.EncodeToString([]byte(key)), "provider": record.ProviderID, "subject": record.Subject})
		}
		return identities
	}
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

func (s *AuthService) identityProviderNames(userID string) []string {
	s.mu.RLock()
	ssoRepository := s.ssoRepository
	s.mu.RUnlock()
	providers := []string{}
	if ssoRepository != nil {
		providers = s.identityProviderNamesFromRepository(ssoRepository, userID)
	} else {
		providers = s.identityProviderNamesFromMemory(userID)
	}
	sort.Strings(providers)
	return providers
}

func (s *AuthService) identityProviderNamesFromRepository(repository store.SSORepository, userID string) []string {
	records, err := repository.ListIdentities(context.Background(), userID)
	if err != nil {
		return []string{}
	}
	seen := map[string]bool{}
	providers := []string{}
	for _, record := range records {
		if !seen[record.ProviderID] {
			seen[record.ProviderID] = true
			providers = append(providers, record.ProviderID)
		}
	}
	return providers
}

func (s *AuthService) identityProviderNamesFromMemory(userID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	providers := []string{}
	for key, owner := range s.oidcIdentities {
		if owner != userID {
			continue
		}
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) == 2 && !seen[parts[0]] {
			seen[parts[0]] = true
			providers = append(providers, parts[0])
		}
	}
	return providers
}

func (s *AuthService) Users(statusFilter ...string) ([]map[string]any, error) {
	status := ""
	if len(statusFilter) > 0 {
		status = statusFilter[0]
	}
	records, err := s.users.List(context.Background(), status)
	if err != nil {
		return nil, err
	}
	return s.authUsers(records)
}

func (s *AuthService) UsersPage(status, email string, roles []string, limit, offset int) ([]map[string]any, int, bool, error) {
	repository, ok := s.users.(interface {
		ListPage(context.Context, string, string, []string, int, int) ([]store.UserRecord, int, error)
	})
	if !ok {
		return nil, 0, false, nil
	}
	records, total, err := repository.ListPage(context.Background(), status, email, roles, limit, offset)
	if err != nil {
		return nil, 0, true, err
	}
	users, err := s.authUsers(records)
	return users, total, true, err
}

func (s *AuthService) authUsers(records []store.UserRecord) ([]map[string]any, error) {
	users := make([]map[string]any, 0, len(records))
	for _, record := range records {
		user := toAuthUser(record)
		userRoles, assignments, err := s.roles.UserRoles(context.Background(), user.ID)
		if err != nil {
			continue
		}
		roles := make([]string, 0, len(userRoles))
		for _, role := range userRoles {
			roles = append(roles, role.Name)
		}
		sort.Strings(roles)
		methods := []string{}
		if hash, hasHash, _ := s.users.PasswordHash(context.Background(), user.ID); hasHash && hash != "" {
			methods = append(methods, "password")
		}
		methods = append(methods, s.identityProviderNames(user.ID)...)
		s.mu.RLock()
		systemAdmin := s.systemAdminEmails[user.Email] || hasSystemAdminAssignment(userRoles, assignments)
		s.mu.RUnlock()
		sort.Strings(methods)
		sessions, err := s.sessions.List(user.ID)
		if err != nil {
			return nil, err
		}
		users = append(users, map[string]any{"id": user.ID, "username": user.Username, "email": user.Email, "displayName": user.DisplayName, "enabled": user.Enabled, "systemAdmin": systemAdmin, "status": user.Status, "roles": roles, "loginMethods": methods, "sessions": sessions})
	}
	sort.Slice(users, func(i, j int) bool { return users[i]["username"].(string) < users[j]["username"].(string) })
	return users, nil
}

func (s *AuthService) AdminSessionsPage(email string, limit, offset int) ([]AdminSession, int, bool, error) {
	sessions, total, handled, err := s.sessions.AdminPage(email, limit, offset)
	return sessions, total, handled, err
}

func (s *AuthService) AdminSessions() ([]AdminSession, error) {
	records, err := s.users.List(context.Background(), "")
	if err != nil {
		return nil, err
	}
	users := make(map[string]string, len(records))
	for _, record := range records {
		users[record.ID] = record.Email
	}
	sessions := []AdminSession{}
	for userID, email := range users {
		userSessions, err := s.sessions.List(userID)
		if err != nil {
			return nil, err
		}
		for _, session := range userSessions {
			sessions = append(sessions, AdminSession{ID: session.ID, UserID: userID, UserEmail: email, ExpiresAt: session.ExpiresAt, LastSeenAt: session.LastSeenAt, UserAgent: session.UserAgent, IPAddress: session.IPAddress})
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UserEmail < sessions[j].UserEmail || sessions[i].UserEmail == sessions[j].UserEmail && sessions[i].ID < sessions[j].ID
	})
	return sessions, nil
}

func (s *AuthService) issueTokens(userID string) (AuthTokens, error) {
	s.mu.RLock()
	repository := s.sessionRepository
	accessLifetime, refreshLifetime := s.accessLifetime, s.refreshLifetime
	s.mu.RUnlock()
	if repository != nil {
		sessionID, err := randomID()
		if err != nil {
			return AuthTokens{}, err
		}
		refreshToken, err := randomID()
		if err != nil {
			return AuthTokens{}, err
		}
		familyID, err := randomID()
		if err != nil {
			return AuthTokens{}, err
		}
		accessToken, _, err := s.sessions.IssueForSession(userID, sessionID, accessLifetime)
		if err != nil {
			return AuthTokens{}, err
		}
		now := time.Now().UTC()
		if err := repository.Create(context.Background(), store.SessionRecord{ID: sessionID, UserID: userID, RefreshTokenHash: platform.HashToken(refreshToken), AccessExpiresAt: now.Add(accessLifetime), RefreshExpiresAt: now.Add(refreshLifetime), SessionFamilyID: familyID, LastSeenAt: now}); err != nil {
			return AuthTokens{}, err
		}
		return AuthTokens{AccessToken: accessToken, RefreshToken: refreshToken, SessionID: sessionID}, nil
	}
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
func (s *AuthService) Refresh(sessionID, refreshToken string) (AuthTokens, error) {
	s.mu.RLock()
	repository := s.sessionRepository
	accessLifetime, refreshLifetime := s.accessLifetime, s.refreshLifetime
	audit := s.audit
	s.mu.RUnlock()
	if repository != nil {
		current, ok, err := repository.Get(context.Background(), sessionID)
		if err != nil || !ok {
			return AuthTokens{}, errors.New("refresh token is invalid")
		}
		newID, err := randomID()
		if err != nil {
			return AuthTokens{}, err
		}
		newToken, err := randomID()
		if err != nil {
			return AuthTokens{}, err
		}
		access, _, err := s.sessions.IssueForSession(current.UserID, newID, accessLifetime)
		if err != nil {
			return AuthTokens{}, err
		}
		now := time.Now().UTC()
		err = repository.Rotate(context.Background(), sessionID, platform.HashToken(refreshToken), store.SessionRecord{ID: newID, RefreshTokenHash: platform.HashToken(newToken), AccessExpiresAt: now.Add(accessLifetime), RefreshExpiresAt: now.Add(refreshLifetime), LastSeenAt: now})
		if err != nil {
			if errors.Is(err, store.ErrSessionReplay) && audit != nil {
				audit(current.UserID, "auth.refresh.replay", sessionID)
			}
			return AuthTokens{}, errors.New("refresh token is invalid")
		}
		return AuthTokens{AccessToken: access, RefreshToken: newToken, SessionID: newID}, nil
	}
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
	if err := s.sessions.Revoke(sessionID); err != nil {
		return AuthTokens{}, err
	}
	return AuthTokens{AccessToken: access, RefreshToken: newToken, SessionID: newID}, nil
}

func (s *AuthService) Logout(sessionID string) error {
	s.refresh.Revoke(sessionID)
	return s.sessions.Revoke(sessionID)
}
func (s *AuthService) LogoutAll(userID string) error {
	s.refresh.RevokeUser(userID)
	return s.sessions.RevokeUser(userID)
}
func (s *AuthService) DisableUser(userID string) error {
	user, ok, err := s.userByID(userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(errorUserNotFound)
	}
	userRoles, assignments, err := s.roles.UserRoles(context.Background(), userID)
	if err != nil {
		return err
	}
	s.mu.RLock()
	systemAdmin := s.systemAdminEmails[user.Email] || hasSystemAdminAssignment(userRoles, assignments)
	s.mu.RUnlock()
	isAdmin := false
	for _, role := range userRoles {
		if role.Name == "admin" {
			isAdmin = true
			break
		}
	}
	if systemAdmin {
		return platform.ErrSystemAdministrator
	}
	disable := func() error {
		if err := s.users.SetEnabled(context.Background(), userID, false); err != nil {
			return err
		}
		return s.LogoutAll(userID)
	}
	if isAdmin {
		return s.adminGuard.Remove(userID, disable)
	}
	return disable()
}

func (s *AuthService) ApproveUser(userID string) error {
	user, ok, err := s.userByID(userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(errorUserNotFound)
	}
	if user.Status == store.StatusActive {
		return nil
	}
	if err := s.users.SetStatus(context.Background(), userID, store.StatusActive); err != nil {
		return err
	}
	roles, assignments, err := s.roles.UserRoles(context.Background(), userID)
	if err != nil {
		return err
	}
	s.mu.RLock()
	systemAdmin := s.systemAdminEmails[user.Email] || hasSystemAdminAssignment(roles, assignments)
	s.mu.RUnlock()
	for _, role := range roles {
		if role.Name == "admin" || systemAdmin {
			s.adminGuard.Add(userID)
			break
		}
	}
	return s.LogoutAll(userID)
}

func (s *AuthService) Revoke(userID, role string) error {
	user, ok, err := s.userByID(userID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New(errorUserNotFound)
	}
	definition, roleFound, err := s.roles.FindByName(context.Background(), role)
	if err != nil {
		return err
	}
	if !roleFound {
		return errors.New("role not assigned")
	}
	userRoles, assignments, err := s.roles.UserRoles(context.Background(), userID)
	if err != nil {
		return err
	}
	s.mu.RLock()
	systemAdmin := s.systemAdminEmails[user.Email] || hasSystemAdminAssignment(userRoles, assignments)
	s.mu.RUnlock()
	assigned := false
	for _, assignedRole := range userRoles {
		if assignedRole.ID == definition.ID {
			assigned = true
			break
		}
	}
	if role == "admin" && systemAdmin {
		return platform.ErrSystemAdministrator
	}
	if !assigned {
		return errors.New("role not assigned")
	}
	if role == "admin" {
		return s.adminGuard.Remove(userID, func() error {
			s.mu.RLock()
			if s.systemAdminEmails[user.Email] {
				s.mu.RUnlock()
				return platform.ErrSystemAdministrator
			}
			s.mu.RUnlock()
			return s.roles.Unassign(context.Background(), userID, definition.ID)
		})
	}
	return s.roles.Unassign(context.Background(), userID, definition.ID)
}
func (s *AuthService) Permissions(claims Claims) map[string]bool {
	out := map[string]bool{}
	permissions, err := s.roles.EffectivePermissions(context.Background(), claims.UserID)
	if err != nil {
		return out
	}
	for _, permission := range permissions {
		out[permission] = true
	}
	return out
}
func (s *AuthService) Authenticator() Authenticator {
	return func(r *http.Request) (Claims, bool) {
		claims, ok := s.sessions.Authenticator()(r)
		if !ok {
			return Claims{}, false
		}
		user, exists, _ := s.userByID(claims.UserID)
		if !exists || !user.Enabled {
			return Claims{}, false
		}
		return claims, true
	}
}
func (s *AuthService) User(userID string) (AuthUser, bool) {
	user, ok, _ := s.userByID(userID)
	return user, ok
}

func (s *AuthService) Profile(claims Claims) (map[string]any, error) {
	user, _, _ := s.userByID(claims.UserID)
	rolesForUser, assignments, _ := s.roles.UserRoles(context.Background(), claims.UserID)
	assignmentSource := map[string]string{}
	for _, assignment := range assignments {
		assignmentSource[assignment.RoleID] = assignment.SourceType
	}
	s.mu.RLock()
	systemAdmin := s.systemAdminEmails[user.Email] || hasSystemAdminAssignment(rolesForUser, assignments)
	roles := []string{}
	roleSources := []string{}
	permissions := map[string]bool{}
	for _, role := range rolesForUser {
		roles = append(roles, role.Name)
		source := assignmentSource[role.ID]
		if source == "" {
			source = "assigned"
		}
		if role.Name == "admin" && systemAdmin {
			source = systemAdminSource
		} else if role.System {
			source = "system"
		}
		roleSources = append(roleSources, role.Name+":"+source)
		for _, permission := range role.Permissions {
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
	sessions, err := s.sessions.List(claims.UserID)
	if err != nil {
		return nil, err
	}
	for index := range sessions {
		sessions[index].Current = sessions[index].ID == claims.SessionID
	}
	methods := []string{}
	if hash, hasHash, _ := s.users.PasswordHash(context.Background(), claims.UserID); hasHash && hash != "" {
		methods = append(methods, "password")
	}
	methods = append(methods, s.identityProviderNames(claims.UserID)...)
	sort.Strings(methods)
	return map[string]any{"id": user.ID, "username": user.Username, "email": user.Email, "displayName": user.DisplayName, "enabled": user.Enabled, "systemAdmin": systemAdmin, "status": user.Status, "roles": roles, "roleSources": roleSources, "permissions": permissionKeys, "loginMethods": methods, "sessions": sessions, "identities": s.Identities(claims.UserID)}, nil
}

func (s *AuthService) UserProfile(userID string) (map[string]any, bool, error) {
	_, ok, _ := s.userByID(userID)
	if !ok {
		return nil, false, nil
	}
	profile, err := s.Profile(Claims{UserID: userID})
	return profile, true, err
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
