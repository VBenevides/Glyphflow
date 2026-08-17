package platform

import "testing"

func TestSSORoleReconcilePreservesManualAndRemovesOnlyProvider(t *testing.T) {
	existing := []RoleAssignment{
		{RoleID: "manual", SourceType: "manual", SourceKey: "owner"},
		{RoleID: "old", SourceType: "sso", SourceKey: "old-group", ProviderID: "p1"},
		{RoleID: "other", SourceType: "sso", SourceKey: "keep", ProviderID: "p2"},
	}
	got := ReconcileSSOAssignments(existing, "p1", []string{"new-group", "new-group"}, map[string]string{"new-group": "new-role"})
	if len(got) != 3 {
		t.Fatalf("unexpected assignments: %#v", got)
	}
	for _, a := range got {
		if a.RoleID == "manual" || a.ProviderID == "p2" {
			continue
		}
		if a.RoleID != "new-role" {
			t.Fatalf("obsolete provider assignment remained: %#v", a)
		}
	}
}
