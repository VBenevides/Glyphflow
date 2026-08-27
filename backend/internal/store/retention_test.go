package store

import (
	"context"
	"errors"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestDefaultRetentionPolicy(t *testing.T) {
	policy := DefaultRetentionPolicy()
	if policy.LogMonthsKeep != 3 || policy.AuditMonthsKeep != 12 || policy.RunnerMetricsMonthsKeep != 3 || !policy.valid() {
		t.Fatalf("default retention policy = %#v", policy)
	}
}

func TestBoundedRetentionBatch(t *testing.T) {
	if got := boundedRetentionBatch(0); got != defaultRetentionBatch {
		t.Fatalf("default batch = %d", got)
	}
	if got := boundedRetentionBatch(maxRetentionBatch + 1); got != defaultRetentionBatch {
		t.Fatalf("oversized batch = %d", got)
	}
	if got := boundedRetentionBatch(25); got != 25 {
		t.Fatalf("valid batch = %d", got)
	}
}

func TestRunStoreRejectsEmergencyStorageBeforeDatabaseAccess(t *testing.T) {
	store := &RunStore{storagePressure: func(context.Context) (platform.StoragePressure, error) {
		return platform.StoragePressure{State: platform.StorageEmergency, Code: "storage_exhausted"}, nil
	}}
	if err := store.ensureStorageAvailable(context.Background()); !errors.Is(err, ErrStorageExhausted) {
		t.Fatalf("storage guard error = %v", err)
	}
}
