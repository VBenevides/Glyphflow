package worker

import "testing"

func TestLocalStoreDeduplicatesOrders(t *testing.T) {
	s, err := OpenStore(t.TempDir() + "/worker.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("order-1", map[string]string{"state": "accepted"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("order-1", map[string]string{"state": "started"}); err != nil {
		t.Fatal(err)
	}
	if string(s.Messages["order-1"]) != `{"state":"accepted"}` {
		t.Fatal("duplicate order changed durable state")
	}
}
