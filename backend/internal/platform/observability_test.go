package platform

import "testing"

func TestMetricsExposeOnlyFixedLowCardinalityNames(t *testing.T) {
	m := new(Metrics)
	m.LoginFailures.Add(2)
	m.PermissionDenials.Add(1)
	snapshot := m.Snapshot()
	if snapshot["login_failures"] != 2 || snapshot["permission_denials"] != 1 {
		t.Fatalf("metrics missing: %#v", snapshot)
	}
}
