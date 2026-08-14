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
	before := map[string]RoleAssignment{}
	for _, assignment := range existing {
		if assignment.UserID == "" {
			assignment.UserID = userID
		}
		before[assignment.UserID+"\x00"+assignment.RoleID+"\x00"+assignment.SourceType+"\x00"+assignment.SourceKey] = assignment
	}
	next := ReconcileSSOAssignments(existing, providerID, groups, groupRoles)
	changes := []AssignmentChange{}
	after := map[string]RoleAssignment{}
	for _, assignment := range next {
		if assignment.UserID == "" {
			assignment.UserID = userID
		}
		key := assignment.UserID + "\x00" + assignment.RoleID + "\x00" + assignment.SourceType + "\x00" + assignment.SourceKey
		after[key] = assignment
		if _, ok := before[key]; !ok && assignment.SourceType == "sso" && assignment.ProviderID == providerID {
			change := AssignmentChange{Action: "added", Assignment: assignment}
			changes = append(changes, change)
			if audit != nil {
				audit(change)
			}
		}
	}
	for key, assignment := range before {
		if _, ok := after[key]; !ok && assignment.SourceType == "sso" && assignment.ProviderID == providerID {
			change := AssignmentChange{Action: "removed", Assignment: assignment}
			changes = append(changes, change)
			if audit != nil {
				audit(change)
			}
		}
	}
	return next, changes
}

func ExtractSSOGroups(claims map[string]any, paths []string) []string {
	seen := map[string]bool{}
	var groups []string
	for _, path := range paths {
		var value any = claims
		for _, part := range strings.Split(path, ".") {
			object, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = object[part]
		}
		switch values := value.(type) {
		case string:
			if values != "" && !seen[values] {
				seen[values] = true
				groups = append(groups, values)
			}
		case []any:
			for _, item := range values {
				if group, ok := item.(string); ok && group != "" && !seen[group] {
					seen[group] = true
					groups = append(groups, group)
				}
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
