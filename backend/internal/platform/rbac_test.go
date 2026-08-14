package platform

import "testing"

func TestSeedRolesAreStableAndAdminHasCatalog(t *testing.T) {
	first, err := SeedRoles()
	if err != nil {
		t.Fatal(err)
	}
	second, err := SeedRoles()
	if err != nil || first[0].ID != second[0].ID || len(first[0].Permissions) != len(PermissionCatalog) || len(first[1].Permissions) != 0 {
		t.Fatalf("seed roles are not idempotent: %#v %#v %v", first, second, err)
	}
}
