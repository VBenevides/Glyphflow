package platform

import "testing"

func TestObjectLogStoreBoundsBodies(t *testing.T) {
	s, err := NewObjectLogStore(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put("log", []byte("12345")); err == nil {
		t.Fatal("oversized log accepted")
	}
	if err := s.Put("log", []byte("1234")); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get("log"); string(got) != "1234" {
		t.Fatalf("unexpected log body: %q", got)
	}
}
