package platform

import (
	"sync"
	"testing"
)

func TestAdministratorGuardPreventsConcurrentLastRemoval(t *testing.T) {
	guard := NewAdministratorGuard("admin-1", "admin-2")
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []string{"admin-1", "admin-2"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			results <- guard.Remove(id, nil)
		}(id)
	}
	wg.Wait()
	close(results)
	var success, last int
	for err := range results {
		if err == nil {
			success++
		}
		if err == ErrLastAdministrator {
			last++
		}
	}
	if success != 1 || last != 1 || guard.ActiveCount() != 1 {
		t.Fatalf("unexpected concurrent removal result: success=%d last=%d active=%d", success, last, guard.ActiveCount())
	}
}

func TestSystemRoleMutationIsRejected(t *testing.T) {
	if err := ValidateRoleMutation(true); err != ErrSystemRole {
		t.Fatalf("system role mutation returned %v", err)
	}
	if err := ValidateRoleMutation(false); err != nil {
		t.Fatal(err)
	}
}
