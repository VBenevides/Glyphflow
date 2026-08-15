package platform

import "testing"

func TestResolveGlobalVariables(t *testing.T) {
	resolved, err := ResolveGlobalVariables("$(PYTHON_PATH)/bin:$(CACHE_PATH)", map[string]string{"PYTHON_PATH": "/opt/python", "CACHE_PATH": "/tmp/cache"})
	if err != nil || resolved != "/opt/python/bin:/tmp/cache" {
		t.Fatalf("resolved = %q, err = %v", resolved, err)
	}
	if _, err := ResolveGlobalVariables("$(MISSING)", nil); err == nil {
		t.Fatal("undefined variable was accepted")
	}
}

func TestGlobalVariableName(t *testing.T) {
	if !GlobalVariableName("CACHE_PATH") || GlobalVariableName("CACHE-PATH") || GlobalVariableName("") {
		t.Fatal("global variable name validation failed")
	}
}
