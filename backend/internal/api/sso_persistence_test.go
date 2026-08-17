package api

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type memorySSORepository struct {
	mu         sync.Mutex
	identities map[string]store.SSOIdentityRecord
	mappings   map[string][]store.SSOGroupRoleMappingRecord
}

func newMemorySSORepository() *memorySSORepository {
	return &memorySSORepository{identities: map[string]store.SSOIdentityRecord{}, mappings: map[string][]store.SSOGroupRoleMappingRecord{}}
}

func (r *memorySSORepository) FindIdentity(_ context.Context, provider, subject string) (store.SSOIdentityRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	identity, ok := r.identities[provider+"\x00"+subject]
	return identity, ok, nil
}

func (r *memorySSORepository) ListIdentities(_ context.Context, userID string) ([]store.SSOIdentityRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	identities := []store.SSOIdentityRecord{}
	for _, identity := range r.identities {
		if identity.UserID == userID {
			identities = append(identities, identity)
		}
	}
	return identities, nil
}

func (r *memorySSORepository) CreateIdentity(_ context.Context, identity store.SSOIdentityRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := identity.ProviderID + "\x00" + identity.Subject
	if _, exists := r.identities[key]; exists {
		return errors.New("identity already linked")
	}
	r.identities[key] = identity
	return nil
}

func (r *memorySSORepository) DeleteIdentity(_ context.Context, userID, provider, subject string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := provider + "\x00" + subject
	identity, ok := r.identities[key]
	if !ok || identity.UserID != userID {
		return errors.New("identity not found")
	}
	delete(r.identities, key)
	return nil
}

func (r *memorySSORepository) ListGroupRoleMappings(_ context.Context, provider string) ([]store.SSOGroupRoleMappingRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]store.SSOGroupRoleMappingRecord(nil), r.mappings[provider]...), nil
}

func (r *memorySSORepository) SetGroupRoleMapping(_ context.Context, mapping store.SSOGroupRoleMappingRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mappings[mapping.ProviderID] = append(r.mappings[mapping.ProviderID], mapping)
	return nil
}

func (r *memorySSORepository) DeleteGroupRoleMapping(_ context.Context, provider, group, role string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	mappings := r.mappings[provider]
	for i, mapping := range mappings {
		if mapping.GroupName == group && mapping.RoleID == role {
			r.mappings[provider] = append(mappings[:i], mappings[i+1:]...)
			return nil
		}
	}
	return errors.New("mapping not found")
}

func TestOIDCIdentityAndGroupRoleAssignmentsUseDurableRepositories(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("operator", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	repository := newMemorySSORepository()
	auth.SetSSORepository(repository)
	user, err := auth.Register("alice@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.LinkOIDC(user.ID, "corp", "subject"); err != nil {
		t.Fatal(err)
	}
	operator, found, err := auth.roles.FindByName(context.Background(), "operator")
	if err != nil || !found {
		t.Fatalf("operator role = %#v, found = %t, err = %v", operator, found, err)
	}
	if err := repository.SetGroupRoleMapping(context.Background(), store.SSOGroupRoleMappingRecord{ProviderID: "corp", GroupName: "admins", RoleID: operator.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.LoginOIDCWithGroups("corp", "subject", "alice", "alice@example.com", false, []string{"admins"}); err != nil {
		t.Fatal(err)
	}
	roles, assignments, err := auth.roles.UserRoles(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	hasOperator, hasSSO := false, false
	for _, role := range roles {
		hasOperator = hasOperator || role.ID == operator.ID
	}
	for _, assignment := range assignments {
		hasSSO = hasSSO || assignment.RoleID == operator.ID && assignment.SourceType == "sso"
	}
	if !hasOperator || !hasSSO {
		t.Fatalf("group role assignment missing: roles=%#v assignments=%#v", roles, assignments)
	}
	if _, err := auth.LoginOIDCWithGroups("corp", "subject", "alice", "alice@example.com", false, nil); err != nil {
		t.Fatal(err)
	}
	_, assignments, _ = auth.roles.UserRoles(context.Background(), user.ID)
	for _, assignment := range assignments {
		if assignment.SourceType == "sso" {
			t.Fatalf("stale SSO assignment remained: %#v", assignments)
		}
	}
	identities := auth.Identities(user.ID)
	if len(identities) != 1 {
		t.Fatalf("identities = %#v", identities)
	}
	if err := auth.UnlinkOIDC(user.ID, identities[0]["id"].(string)); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.LoginOIDC("corp", "subject", "alice", "alice@example.com", false); err == nil {
		t.Fatal("unlinked OIDC identity still authenticated")
	}
}
