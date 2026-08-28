package platform

import (
	"math"
	"sync"
)

type StorageState string

const (
	StorageNormal      StorageState = "NORMAL"
	StorageWarning     StorageState = "WARNING"
	StorageCritical    StorageState = "CRITICAL"
	StorageEmergency   StorageState = "EMERGENCY"
	StorageUnavailable StorageState = "UNAVAILABLE"
)

type StoragePressure struct {
	State       StorageState `json:"state"`
	Code        string       `json:"code,omitempty"`
	FreeBytes   uint64       `json:"freeBytes"`
	FreePercent float64      `json:"freePercent"`
}

func (p StoragePressure) RejectNewRuns() bool {
	return p.State == StorageEmergency || p.State == StorageUnavailable
}

func EvaluateStoragePressure(freePercent float64) StoragePressure {
	if math.IsNaN(freePercent) || math.IsInf(freePercent, 0) || freePercent < 0 {
		return StoragePressure{State: StorageUnavailable, Code: "storage_unavailable"}
	}
	switch {
	case freePercent <= 5:
		return StoragePressure{State: StorageEmergency, Code: "storage_exhausted", FreePercent: freePercent}
	case freePercent <= 10:
		return StoragePressure{State: StorageCritical, FreePercent: freePercent}
	case freePercent <= 20:
		return StoragePressure{State: StorageWarning, FreePercent: freePercent}
	default:
		return StoragePressure{State: StorageNormal, FreePercent: freePercent}
	}
}

type StoragePressureMonitor struct {
	mu    sync.Mutex
	state StorageState
}

func (m *StoragePressureMonitor) Observe(freePercent float64) StoragePressure {
	if m == nil {
		return EvaluateStoragePressure(freePercent)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	pressure := EvaluateStoragePressure(freePercent)
	if m.state == StorageEmergency && pressure.State != StorageUnavailable && freePercent <= 10 {
		pressure = StoragePressure{State: StorageEmergency, Code: "storage_exhausted", FreePercent: freePercent}
	}
	m.state = pressure.State
	return pressure
}
