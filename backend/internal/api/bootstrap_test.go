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

func TestEnsureBootstrapEnforcesPasswordPolicy(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, []byte("0123456789012345"))
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("admin"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	if _, err := auth.EnsureBootstrap("admin", "short", "", ""); err == nil {
		t.Fatal("short bootstrap password accepted")
	}
	if _, err := auth.EnsureBootstrap("admin", "correct horse", "", ""); err != nil {
		t.Fatal(err)
	}
}
