package platform

import "testing"

func TestSignalBusSupportsNotificationApprovalAndTelemetryKinds(t *testing.T) {
	b := NewSignalBus()
	for _, kind := range []string{"notification", "approval", "runner.telemetry"} {
		ch := b.Subscribe(kind, 1)
		b.Publish(Signal{Kind: kind, ID: "1"})
		select {
		case got := <-ch:
			if got.Kind != kind {
				t.Fatalf("wrong signal: %#v", got)
			}
		default:
			t.Fatalf("signal %s was not delivered", kind)
		}
	}
}
