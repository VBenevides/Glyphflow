package platform

import "testing"

func TestSeedRolesAreStableAndAdminHasCatalog(t *testing.T) {
	first, err := SeedRoles()
	if err != nil {
		t.Fatal(err)
	}
	second, err := SeedRoles()
	if err != nil || first[0].ID != second[0].ID || len(first) != 3 || len(first[0].Permissions) != len(PermissionCatalog) {
		t.Fatalf("seed roles are not idempotent: %#v %#v %v", first, second, err)
	}
	roles := map[string]SeedRole{}
	for _, role := range first {
		roles[role.Key] = role
	}
	for _, permission := range UserPermissionCatalog {
		if !containsPermission(roles["user"].Permissions, permission) {
			t.Fatalf("user role is missing %q", permission)
		}
	}
	for _, permission := range []string{"tasks.manage", "resources.manage", "runners.manage", "audit.read", "users.read", "roles.read", "sso.read", "auth.settings.manage"} {
		if containsPermission(roles["user"].Permissions, permission) {
			t.Fatalf("user role has restricted permission %q", permission)
		}
	}
	for _, permission := range OperatorPermissionCatalog {
		if !containsPermission(roles["operator"].Permissions, permission) {
			t.Fatalf("operator role is missing %q", permission)
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
