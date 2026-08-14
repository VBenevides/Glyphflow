package platform

import "testing"

func TestExtractSSOGroupsFromConfiguredClaimPaths(t *testing.T) {
	claims := map[string]any{"groups": []any{"ops", "ops"}, "realm": map[string]any{"groups": "admins"}}
	groups := ExtractSSOGroups(claims, []string{"groups", "realm.groups"})
	if len(groups) != 2 || groups[0] != "ops" || groups[1] != "admins" {
		t.Fatalf("groups were not extracted: %#v", groups)
	}
}
