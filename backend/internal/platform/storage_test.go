package platform

import "testing"

func TestStoragePressureThresholds(t *testing.T) {
	for _, test := range []struct {
		free  float64
		state StorageState
		code  string
	}{
		{25, StorageNormal, ""},
		{20, StorageWarning, ""},
		{10, StorageCritical, ""},
		{5, StorageEmergency, "storage_exhausted"},
	} {
		got := EvaluateStoragePressure(test.free)
		if got.State != test.state || got.Code != test.code {
			t.Fatalf("free=%v: got %#v", test.free, got)
		}
	}
}

func TestStorageEmergencyRecoversOnlyAboveTenPercent(t *testing.T) {
	monitor := new(StoragePressureMonitor)
	if got := monitor.Observe(5); got.State != StorageEmergency {
		t.Fatalf("initial state = %#v", got)
	}
	if got := monitor.Observe(10); got.State != StorageEmergency {
		t.Fatalf("state at recovery boundary = %#v", got)
	}
	if got := monitor.Observe(10.1); got.State != StorageWarning {
		t.Fatalf("state after recovery = %#v", got)
	}
}
