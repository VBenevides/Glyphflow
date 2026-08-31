package store

import (
	"context"
	"errors"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestDatabaseStoragePressureUsesDatabaseSize(t *testing.T) {
	used := int64(25)
	provider := NewDatabaseStoragePressureProvider(func(context.Context) (int64, error) { return used, nil }, 100)
	pressure, err := provider(context.Background())
	if err != nil || pressure.State != platform.StorageNormal || pressure.FreeBytes != 75 {
		t.Fatalf("normal pressure = %#v, err=%v", pressure, err)
	}
	used = 90
	pressure, err = provider(context.Background())
	if err != nil || pressure.State != platform.StorageCritical || pressure.FreeBytes != 10 {
		t.Fatalf("critical pressure = %#v, err=%v", pressure, err)
	}
}

func TestDatabaseStoragePressureFailsClosedWithoutCapacity(t *testing.T) {
	provider := NewDatabaseStoragePressureProvider(func(context.Context) (int64, error) {
		t.Fatal("database size should not be queried without capacity")
		return 0, nil
	}, 0)
	pressure, err := provider(context.Background())
	if err != nil || pressure.State != platform.StorageUnavailable || !pressure.RejectNewRuns() {
		t.Fatalf("unconfigured pressure = %#v, err=%v", pressure, err)
	}
	provider = NewDatabaseStoragePressureProvider(func(context.Context) (int64, error) { return 0, errors.New("database unavailable") }, 100)
	if _, err := provider(context.Background()); err == nil {
		t.Fatal("database size failure was ignored")
	}
}
