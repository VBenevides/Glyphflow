package platform

import "testing"

func TestLoginMethodAndIdentityPolicies(t *testing.T) {
	if err := ValidateLoginMethodRemoval(true, 0, true); err == nil {
		t.Fatal("last login method removal accepted")
	}
	if err := ValidateLoginMethodRemoval(true, 1, true); err != nil {
		t.Fatal(err)
	}
	if NormalizeIdentityKey(" User.Name ") != "user.name" {
		t.Fatal("identity key was not normalized")
	}
}
