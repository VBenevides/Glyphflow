package api

import "testing"

func TestEnsureBootstrapSupportsPasswordDisabledSSO(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("admin", "users.manage"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.EnsureBootstrap("Admin", "", "corp", "subject")
	if err != nil {
		t.Fatal(err)
	}
	if !auth.Permissions(Claims{UserID: user.ID})["users.manage"] {
		t.Fatal("bootstrap admin role missing")
	}
	if _, err := auth.EnsureBootstrap("", "", "", ""); err == nil {
		t.Fatal("empty bootstrap accepted")
	}
}
