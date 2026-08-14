package platform

import "testing"

func TestServiceAccountTokensAreRotatableAndHashed(t *testing.T) {
	s := NewServiceAccountStore()
	first, _ := s.Issue("worker")
	if !s.Verify("worker", first) {
		t.Fatal("issued token rejected")
	}
	second, _ := s.Issue("worker")
	if s.Verify("worker", first) || !s.Verify("worker", second) {
		t.Fatal("token rotation failed")
	}
}
