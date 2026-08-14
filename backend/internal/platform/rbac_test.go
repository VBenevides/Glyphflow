package platform

import "testing"

func TestSeedRolesAreStableAndAdminHasCatalog(t *testing.T) {
	first, err := SeedRoles()
	if err != nil {
		t.Fatal(err)
	}
	second, err := SeedRoles()
	if err != nil || first[0].ID != second[0].ID || len(first[0].Permissions) != len(PermissionCatalog) || len(first[1].Permissions) != len(UserPermissionCatalog) {
		t.Fatalf("seed roles are not idempotent: %#v %#v %v", first, second, err)
	}
	for _, permission := range UserPermissionCatalog {
		if !containsPermission(first[1].Permissions, permission) {
			t.Fatalf("user role is missing %q", permission)
		}
	}
	for _, permission := range []string{"audit.read", "users.read", "roles.read", "sso.read", "auth.settings.manage"} {
		if containsPermission(first[1].Permissions, permission) {
			t.Fatalf("user role has admin-only permission %q", permission)
		}
	}
}

func containsPermission(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
