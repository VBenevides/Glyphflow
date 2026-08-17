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

func TestLeaseExpiryRenewalAndReleaseStates(t *testing.T) {
	m := NewLeaseManager()
	now := time.Now()
	lease, err := m.Acquire("resource", "attempt", "token", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Renew(lease, now.Add(2*time.Minute), time.Minute); err == nil {
		t.Fatal("expired lease renewed")
	}
	expired := m.Expire(now.Add(2 * time.Minute))
	if len(expired) != 1 || expired[0].State != "EXPIRED" {
		t.Fatalf("expiry state: %#v", expired)
	}
	next, err := m.Acquire("resource", "attempt-2", "token-2", now.Add(2*time.Minute), time.Minute)
	if err != nil || next.Fencing <= lease.Fencing {
		t.Fatalf("takeover fencing: %#v %v", next, err)
	}
	if err := m.Release(next, now); err != nil {
		t.Fatal(err)
	}
	if err := m.Release(next, now); err == nil {
		t.Fatal("released lease released twice")
	}
}
