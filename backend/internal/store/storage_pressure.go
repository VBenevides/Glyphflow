package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/jackc/pgx/v5/pgxpool"
)

const databaseSizeQuery = `SELECT pg_database_size(current_database())`

func NewPostgreSQLStoragePressureProvider(pool *pgxpool.Pool, capacityBytes int64) func(context.Context) (platform.StoragePressure, error) {
	if pool == nil {
		return NewDatabaseStoragePressureProvider(nil, capacityBytes)
	}
	return NewDatabaseStoragePressureProvider(func(ctx context.Context) (int64, error) {
		var usedBytes int64
		if err := pool.QueryRow(ctx, databaseSizeQuery).Scan(&usedBytes); err != nil {
			return 0, err
		}
		return usedBytes, nil
	}, capacityBytes)
}

func NewSQLiteStoragePressureProvider(db *sql.DB, capacityBytes int64) func(context.Context) (platform.StoragePressure, error) {
	if db == nil {
		return NewDatabaseStoragePressureProvider(nil, capacityBytes)
	}
	return NewDatabaseStoragePressureProvider(func(ctx context.Context) (int64, error) {
		var usedBytes int64
		if err := db.QueryRowContext(ctx, `SELECT page_count * page_size FROM pragma_page_count, pragma_page_size`).Scan(&usedBytes); err != nil {
			return 0, err
		}
		return usedBytes, nil
	}, capacityBytes)
}

func NewDatabaseStoragePressureProvider(readSize func(context.Context) (int64, error), capacityBytes int64) func(context.Context) (platform.StoragePressure, error) {
	monitor := new(platform.StoragePressureMonitor)
	return func(ctx context.Context) (platform.StoragePressure, error) {
		if capacityBytes <= 0 {
			return platform.StoragePressure{State: platform.StorageUnavailable, Code: "database_storage_capacity_unconfigured"}, nil
		}
		if readSize == nil {
			return platform.StoragePressure{}, errors.New("database storage size provider is unavailable")
		}
		usedBytes, err := readSize(ctx)
		if err != nil {
			return platform.StoragePressure{}, err
		}
		if usedBytes < 0 {
			return platform.StoragePressure{State: platform.StorageUnavailable, Code: "database_storage_size_invalid"}, nil
		}
		freeBytes := int64(0)
		if usedBytes < capacityBytes {
			freeBytes = capacityBytes - usedBytes
		}
		pressure := monitor.Observe(float64(freeBytes) * 100 / float64(capacityBytes))
		pressure.FreeBytes = uint64(freeBytes)
		return pressure, nil
	}
}
