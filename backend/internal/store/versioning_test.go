package store

import "testing"

func TestVersionRegistryActivatesOneCurrentVersion(t *testing.T) {
	registry := NewVersionRegistry()
	if err := registry.Activate("task-1", "version-1"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Activate("task-1", "version-2"); err != nil || registry.Current("task-1") != "version-2" {
		t.Fatalf("version activation failed: %v", err)
	}
}
