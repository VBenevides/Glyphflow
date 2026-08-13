package queue

import (
	"testing"
)

func TestQueueSubjects(t *testing.T) {
	if Subject("orders", "worker-1") != "glyphflow.orders.worker-1" {
		t.Fatal("unexpected order subject")
	}
	if Subject("events", "worker-1") != "glyphflow.events.worker-1" {
		t.Fatal("unexpected event subject")
	}
}

func TestMutualTLSAndWorkerPermissions(t *testing.T) {
	if _, err := (TLSConfig{}).options(); err == nil {
		t.Fatal("incomplete TLS configuration was accepted")
	}
	permissions := WorkerPermissions("worker-1")
	if len(permissions.Publish.Allow) != 1 || permissions.Publish.Allow[0] != "glyphflow.events.worker-1" {
		t.Fatalf("unexpected publish permissions: %#v", permissions.Publish.Allow)
	}
	if len(permissions.Subscribe.Allow) != 1 || permissions.Subscribe.Allow[0] != "glyphflow.orders.worker-1" {
		t.Fatalf("unexpected subscribe permissions: %#v", permissions.Subscribe.Allow)
	}
	if AllowedWorkerSubject("glyphflow.orders.worker-2", "worker-1") || !AllowedWorkerSubject("glyphflow.events.worker-1", "worker-1") {
		t.Fatal("worker subject isolation failed")
	}
}

func TestQueueDeliveryDefaults(t *testing.T) {
	if Subject("deadletter", "glyphflow.orders.worker-1") != "glyphflow.deadletter.glyphflow.orders.worker-1" {
		t.Fatal("unexpected dead-letter subject")
	}
}

func TestConnectJetStreamRequiresMutualTLS(t *testing.T) {
	if _, err := ConnectJetStream("nats://localhost:4222"); err == nil {
		t.Fatal("plaintext NATS connection was accepted")
	}
}
