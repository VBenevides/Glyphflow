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

func TestVersionRegistryActivationIsIdempotentAndBatchAtomic(t *testing.T) {
	r := NewVersionRegistry()
	if err := r.Activate("task", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := r.Activate("task", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := r.ActivateMany(map[string]string{"task": "v2", "schedule": "s1"}); err != nil {
		t.Fatal(err)
	}
	if r.Current("task") != "v2" || r.Current("schedule") != "s1" {
		t.Fatal("batch activation failed")
	}
	if err := r.ActivateMany(map[string]string{"ok": "v", "": "bad"}); err == nil || r.Current("ok") != "" {
		t.Fatal("invalid batch partially committed")
	}
}
