package platform

import (
	"testing"
	"time"
)

func TestLeaseTakeoverFencesStaleRelease(t *testing.T) {
	manager := NewLeaseManager()
	now := time.Now()
	first, err := manager.Acquire("resource-1", "attempt-1", "token-1", now, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Acquire("resource-1", "attempt-2", "token-2", now, time.Second); err == nil {
		t.Fatal("active lease was replaced")
	}
	second, err := manager.Acquire("resource-1", "attempt-2", "token-2", now.Add(2*time.Second), time.Second)
	if err != nil || second.Fencing <= first.Fencing {
		t.Fatalf("takeover did not fence: %#v %v", second, err)
	}
	if err := manager.Release(first, now); err == nil {
		t.Fatal("stale lease release succeeded")
	}
	if err := manager.Release(second, now); err != nil {
		t.Fatal(err)
	}
}
