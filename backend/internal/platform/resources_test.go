package platform

import "testing"

func TestCapacityResourceBoundsSharedUse(t *testing.T) {
	r := NewCapacityResource(2)
	if !r.Acquire(2) || r.Acquire(1) {
		t.Fatal("capacity bound failed")
	}
	r.Release(1)
	if !r.Acquire(1) {
		t.Fatal("released capacity unavailable")
	}
}
