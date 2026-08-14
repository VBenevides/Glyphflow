package platform

import "testing"

func TestRoleAssignmentsRequireCanonicalUniqueSource(t *testing.T) {
	s := NewRoleAssignmentStore()
	first := RoleAssignment{UserID: "u", RoleID: "r", SourceType: " Manual ", SourceKey: " Default "}
	if err := s.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(RoleAssignment{UserID: "u", RoleID: "r", SourceType: "manual", SourceKey: "default"}); err == nil {
		t.Fatal("duplicate normalized assignment accepted")
	}
	if err := s.Add(RoleAssignment{UserID: "u", RoleID: "r", SourceType: "system", SourceKey: ""}); err == nil {
		t.Fatal("null source key accepted")
	}
}
