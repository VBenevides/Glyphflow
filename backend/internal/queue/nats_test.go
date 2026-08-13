package queue

import "testing"

func TestQueueSubjects(t *testing.T) {
	if Subject("orders", "worker-1") != "glyphflow.orders.worker-1" {
		t.Fatal("unexpected order subject")
	}
	if Subject("events", "worker-1") != "glyphflow.events.worker-1" {
		t.Fatal("unexpected event subject")
	}
}
