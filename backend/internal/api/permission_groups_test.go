package api

import (
	"strings"
	"testing"
)

func TestRouteRegistryIncludesProtectedEndpointGroups(t *testing.T) {
	if err := ValidateRouteRegistry(RouteRegistry()); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tasks.read", "runs.cancel", "roles.manage", "auth.settings.manage"} {
		found := false
		for _, route := range RouteRegistry() {
			if strings.Contains(route.Permission, key) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing permission group %s", key)
		}
	}
}
