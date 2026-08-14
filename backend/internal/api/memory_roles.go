package api

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type memoryRoleRepository struct {
	mu          sync.RWMutex
	roles       map[string]store.RoleRecord
	byName      map[string]string
	assignments map[string]map[string]store.RoleAssignmentRecord
}

func newMemoryRoleRepository() *memoryRoleRepository {
	return &memoryRoleRepository{roles: map[string]store.RoleRecord{}, byName: map[string]string{}, assignments: map[string]map[string]store.RoleAssignmentRecord{}}
}

func (s *memoryRoleRepository) Ensure(_ context.Context, id, name, description string, system bool, permissions []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.roles[id]; ok {
		existing.Permissions = uniqueStrings(permissions)
		s.roles[id] = existing
		return nil
	}
	name = normalizeRoleName(name)
	if existing := s.byName[name]; existing != "" && existing != id {
		return errors.New("role exists")
	}
	s.roles[id] = store.RoleRecord{ID: id, Name: name, Description: description, System: system, Permissions: uniqueStrings(permissions)}
	s.byName[name] = id
	return nil
}

func (s *memoryRoleRepository) Create(ctx context.Context, id, name, description string, permissions []string) error {
	return s.Ensure(ctx, id, name, description, false, permissions)
}

func (s *memoryRoleRepository) List(_ context.Context) ([]store.RoleRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roles := make([]store.RoleRecord, 0, len(s.roles))
	for _, role := range s.roles {
		role.Permissions = append([]string(nil), role.Permissions...)
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Name < roles[j].Name })
	return roles, nil
}

func (s *memoryRoleRepository) FindByID(_ context.Context, id string) (store.RoleRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	role, ok := s.roles[id]
	return role, ok, nil
}

func (s *memoryRoleRepository) FindByName(_ context.Context, name string) (store.RoleRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	role, ok := s.roles[s.byName[normalizeRoleName(name)]]
	return role, ok, nil
}

func (s *memoryRoleRepository) Rename(_ context.Context, roleID, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	role, ok := s.roles[roleID]
	if !ok {
		return errors.New("role not found")
	}
	if role.System {
		return errors.New("system role is immutable")
	}
	name = normalizeRoleName(name)
	if existing := s.byName[name]; existing != "" && existing != roleID {
		return errors.New("role exists")
	}
	delete(s.byName, role.Name)
	role.Name = name
	s.roles[roleID], s.byName[name] = role, roleID
	return nil
}

func (s *memoryRoleRepository) ReplacePermissions(_ context.Context, roleID string, permissions []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	role, ok := s.roles[roleID]
	if !ok {
		return errors.New("role not found")
	}
	if role.System {
		return errors.New("system role is immutable")
	}
	role.Permissions = uniqueStrings(permissions)
	s.roles[roleID] = role
	return nil
}

func (s *memoryRoleRepository) Delete(_ context.Context, roleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	role, ok := s.roles[roleID]
	if !ok {
		return errors.New("role not found")
	}
	if role.System {
		return errors.New("system role is immutable")
	}
	delete(s.roles, roleID)
	delete(s.byName, role.Name)
	for userID := range s.assignments {
		delete(s.assignments[userID], roleID)
	}
	return nil
}

func (s *memoryRoleRepository) Assign(_ context.Context, userID, roleID, sourceType, sourceKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[roleID]; !ok {
		return errors.New("role not found")
	}
	if s.assignments[userID] == nil {
		s.assignments[userID] = map[string]store.RoleAssignmentRecord{}
	}
	key := sourceType + "\x00" + sourceKey
	s.assignments[userID][roleID+"\x00"+key] = store.RoleAssignmentRecord{UserID: userID, RoleID: roleID, SourceType: sourceType, SourceKey: sourceKey}
	return nil
}

func (s *memoryRoleRepository) Unassign(_ context.Context, userID, roleID string) error {
	return s.UnassignSource(context.Background(), userID, roleID, "manual")
}

func (s *memoryRoleRepository) UnassignSource(_ context.Context, userID, roleID, sourceType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := false
	for key, assignment := range s.assignments[userID] {
		if assignment.RoleID == roleID && assignment.SourceType == sourceType {
			delete(s.assignments[userID], key)
			removed = true
		}
	}
	if !removed {
		return errors.New("assignment not found")
	}
	return nil
}

func (s *memoryRoleRepository) ReplaceSourceAssignments(_ context.Context, roleID, sourceType string, userIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[roleID]; !ok {
		return errors.New("role not found")
	}
	desired := map[string]bool{}
	for _, userID := range uniqueStrings(userIDs) {
		desired[userID] = true
	}
	for userID, assignments := range s.assignments {
		for key, assignment := range assignments {
			if assignment.RoleID == roleID && assignment.SourceType == sourceType {
				delete(assignments, key)
			}
		}
		if len(assignments) == 0 {
			delete(s.assignments, userID)
		}
	}
	for _, userID := range uniqueStrings(userIDs) {
		if s.assignments[userID] == nil {
			s.assignments[userID] = map[string]store.RoleAssignmentRecord{}
		}
		key := roleID + "\x00" + sourceType + "\x00" + userID
		s.assignments[userID][key] = store.RoleAssignmentRecord{UserID: userID, RoleID: roleID, SourceType: sourceType, SourceKey: userID}
	}
	return nil
}

func (s *memoryRoleRepository) ReplaceSSOAssignments(_ context.Context, userID, providerID string, assignments []store.RoleAssignmentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, assignment := range s.assignments[userID] {
		if assignment.SourceType == "sso" && strings.HasPrefix(assignment.SourceKey, store.SSOAssignmentKey(providerID, "")) {
			delete(s.assignments[userID], key)
		}
	}
	for _, assignment := range assignments {
		if assignment.RoleID == "" || assignment.SourceKey == "" {
			continue
		}
		if _, ok := s.roles[assignment.RoleID]; !ok {
			return errors.New("role not found")
		}
		if s.assignments[userID] == nil {
			s.assignments[userID] = map[string]store.RoleAssignmentRecord{}
		}
		key := assignment.RoleID + "\x00sso\x00" + store.SSOAssignmentKey(providerID, assignment.SourceKey)
		s.assignments[userID][key] = store.RoleAssignmentRecord{UserID: userID, RoleID: assignment.RoleID, SourceType: "sso", SourceKey: store.SSOAssignmentKey(providerID, assignment.SourceKey)}
	}
	return nil
}

func (s *memoryRoleRepository) UserRoles(_ context.Context, userID string) ([]store.RoleRecord, []store.RoleAssignmentRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	roles := map[string]store.RoleRecord{}
	assignments := []store.RoleAssignmentRecord{}
	for _, assignment := range s.assignments[userID] {
		assignments = append(assignments, assignment)
		if _, ok := roles[assignment.RoleID]; !ok {
			roles[assignment.RoleID] = s.roles[assignment.RoleID]
		}
	}
	result := make([]store.RoleRecord, 0, len(roles))
	for _, role := range roles {
		result = append(result, role)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, assignments, nil
}

func (s *memoryRoleRepository) EffectivePermissions(_ context.Context, userID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	permissions := map[string]bool{}
	for _, assignment := range s.assignments[userID] {
		for _, permission := range s.roles[assignment.RoleID].Permissions {
			permissions[permission] = true
		}
	}
	result := make([]string, 0, len(permissions))
	for permission := range permissions {
		result = append(result, permission)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeRoleName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
