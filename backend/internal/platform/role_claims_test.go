package platform

import "testing"

func TestExtractSSOGroupsFromConfiguredClaimPaths(t *testing.T) {
	claims := map[string]any{"groups": []any{"ops", "ops"}, "realm": map[string]any{"groups": "admins"}}
	groups := ExtractSSOGroups(claims, []string{"groups", "realm.groups"})
	if len(groups) != 2 || groups[0] != "ops" || groups[1] != "admins" {
		t.Fatalf("groups were not extracted: %#v", groups)
	}
}

func TestSyncSSORolesAuditsOnlyProviderChanges(t *testing.T) {
	existing := []RoleAssignment{{UserID: "u", RoleID: "manual", SourceType: "manual", SourceKey: "owner"}, {UserID: "u", RoleID: "old", SourceType: "sso", SourceKey: "old", ProviderID: "p"}}
	var changes []AssignmentChange
	next, changes := SyncSSORoles("u", "p", existing, []string{"new"}, map[string]string{"new": "new-role"}, func(change AssignmentChange) { changes = append(changes, change) })
	if len(next) != 2 || len(changes) != 2 {
		t.Fatalf("unexpected sync: %#v %#v", next, changes)
	}
}
