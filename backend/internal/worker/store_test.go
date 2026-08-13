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
	defer s.Close()
	value, err := s.Get("order-1")
	if err != nil || string(value) != `{"state":"accepted"}` {
		t.Fatal("duplicate order changed durable state")
	}
}

func TestLocalStoreSurvivesReopen(t *testing.T) {
	path := t.TempDir() + "/worker.sqlite"
	first, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Put("order-1", map[string]string{"state": "accepted"}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := second.Get("order-1"); err != nil {
		t.Fatal(err)
	}
}
