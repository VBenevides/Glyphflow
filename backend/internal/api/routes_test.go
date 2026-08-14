package api

import "testing"

func TestRouteRegistryClassifiesEveryRegisteredRoute(t *testing.T) {
	if err := ValidateRouteRegistry(RouteRegistry()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRouteRegistry([]RouteDefinition{{Pattern: "/unclassified", Access: RoutePermission}}); err == nil {
		t.Fatal("unclassified permission route accepted")
	}
	if err := ValidateBuiltRoutes(map[string]struct{}{"/unclassified": {}}, RouteRegistry()); err == nil {
		t.Fatal("built unclassified route accepted")
	}
}
