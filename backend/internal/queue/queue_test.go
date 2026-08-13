package queue

import (
	"context"
	"testing"
)

func TestMemoryQueueDurableHandoff(t *testing.T) {
	q := NewMemory()
	if err := q.Publish(context.Background(), Message{Subject: "orders.worker-1", ID: "order-1", Data: []byte("signed")}); err != nil {
		t.Fatal(err)
	}
	msg, err := q.Consume(context.Background())
	if err != nil || msg.ID != "order-1" {
		t.Fatalf("unexpected message: %#v %v", msg, err)
	}
}
