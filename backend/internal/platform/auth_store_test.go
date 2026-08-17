package platform

import "testing"

func TestAuthStoreResolvesEffectivePermissions(t *testing.T) {
	s := NewAuthStore()
	if err := s.AddUser(User{ID: "u", Username: "User"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRole(Role{ID: "r", Key: "reader"}, "tasks.read"); err != nil {
		t.Fatal(err)
	}
	if err := s.AssignRole("u", "r"); err != nil {
		t.Fatal(err)
	}
	if !s.EffectivePermissions("u")["tasks.read"] {
		t.Fatal("permission not resolved")
	}
	if err := s.AddUser(User{ID: "u2", Username: " user "}); err == nil {
		t.Fatal("normalized duplicate username accepted")
	}
}
