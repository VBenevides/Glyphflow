package platform

import (
	"errors"
	"strings"
	"sync"
)

type RoleAssignment struct {
	UserID     string
	RoleID     string
	SourceType string
	SourceKey  string
	ProviderID string
	ExternalID string
}

// ReconcileSSOAssignments changes only assignments sourced from one provider.
func ReconcileSSOAssignments(existing []RoleAssignment, providerID string, groups []string, groupRoles map[string]string) []RoleAssignment {
	seen := make(map[string]bool)
	out := make([]RoleAssignment, 0, len(existing)+len(groups))
	for _, assignment := range existing {
		if assignment.SourceType == "sso" && assignment.ProviderID == providerID {
			continue
		}
		out = append(out, assignment)
	}
	for _, group := range groups {
		role, ok := groupRoles[group]
		if !ok || seen[group+"\x00"+role] {
			continue
		}
		seen[group+"\x00"+role] = true
		out = append(out, RoleAssignment{RoleID: role, SourceType: "sso", SourceKey: group, ProviderID: providerID, ExternalID: group})
	}
	return out
}

type AssignmentChange struct {
	Action     string
	Assignment RoleAssignment
}

func SyncSSORoles(userID, providerID string, existing []RoleAssignment, groups []string, groupRoles map[string]string, audit func(AssignmentChange)) ([]RoleAssignment, []AssignmentChange) {
	before := assignmentMap(userID, existing)
	next := ReconcileSSOAssignments(existing, providerID, groups, groupRoles)
	after := assignmentMap(userID, next)
	changes := assignmentChanges(before, after, providerID, audit)
	return next, changes
}

func assignmentMap(userID string, assignments []RoleAssignment) map[string]RoleAssignment {
	result := make(map[string]RoleAssignment, len(assignments))
	for _, assignment := range assignments {
		if assignment.UserID == "" {
			assignment.UserID = userID
		}
		result[assignmentIdentity(assignment)] = assignment
	}
	return result
}

func assignmentIdentity(assignment RoleAssignment) string {
	return assignment.UserID + "\x00" + assignment.RoleID + "\x00" + assignment.SourceType + "\x00" + assignment.SourceKey
}

func assignmentChanges(before, after map[string]RoleAssignment, providerID string, audit func(AssignmentChange)) []AssignmentChange {
	changes := make([]AssignmentChange, 0)
	for key, assignment := range after {
		if _, ok := before[key]; !ok && assignment.SourceType == "sso" && assignment.ProviderID == providerID {
			changes = appendAssignmentChange(changes, AssignmentChange{Action: "added", Assignment: assignment}, audit)
		}
	}
	for key, assignment := range before {
		if _, ok := after[key]; !ok && assignment.SourceType == "sso" && assignment.ProviderID == providerID {
			changes = appendAssignmentChange(changes, AssignmentChange{Action: "removed", Assignment: assignment}, audit)
		}
	}
	return changes
}

func appendAssignmentChange(changes []AssignmentChange, change AssignmentChange, audit func(AssignmentChange)) []AssignmentChange {
	if audit != nil {
		audit(change)
	}
	return append(changes, change)
}

func ExtractSSOGroups(claims map[string]any, paths []string) []string {
	seen := map[string]bool{}
	var groups []string
	for _, path := range paths {
		groups = appendSSOGroups(groups, seen, claimValue(claims, path))
	}
	return groups
}

func claimValue(claims map[string]any, path string) any {
	var value any = claims
	for _, part := range strings.Split(path, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[part]
	}
	return value
}

func appendSSOGroups(groups []string, seen map[string]bool, value any) []string {
	add := func(group string) {
		if group != "" && !seen[group] {
			seen[group] = true
			groups = append(groups, group)
		}
	}
	switch values := value.(type) {
	case string:
		add(values)
	case []any:
		for _, item := range values {
			if group, ok := item.(string); ok {
				add(group)
			}
		}
	}
	return groups
}

type RoleAssignmentStore struct {
	mu    sync.Mutex
	items map[string]RoleAssignment
}

func NewRoleAssignmentStore() *RoleAssignmentStore {
	return &RoleAssignmentStore{items: make(map[string]RoleAssignment)}
}

func CanonicalSourceKey(sourceType, sourceKey string) (string, error) {
	sourceType, sourceKey = NormalizeIdentityKey(sourceType), NormalizeIdentityKey(sourceKey)
	if sourceType == "" || sourceKey == "" {
		return "", errors.New("assignment source type and key are required")
	}
	return sourceType + ":" + sourceKey, nil
}

func (s *RoleAssignmentStore) Add(a RoleAssignment) error {
	if a.UserID == "" || a.RoleID == "" {
		return errors.New("assignment user and role are required")
	}
	key, err := CanonicalSourceKey(a.SourceType, a.SourceKey)
	if err != nil {
		return err
	}
	a.SourceType, a.SourceKey = strings.SplitN(key, ":", 2)[0], strings.SplitN(key, ":", 2)[1]
	identity := a.UserID + "\x00" + a.RoleID + "\x00" + key
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.items[identity]; exists {
		return errors.New("duplicate role assignment")
	}
	s.items[identity] = a
	return nil
}

func (s *RoleAssignmentStore) List(userID string) []RoleAssignment {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []RoleAssignment
	for _, a := range s.items {
		if a.UserID == userID {
			out = append(out, a)
		}
	}
	return out
}
