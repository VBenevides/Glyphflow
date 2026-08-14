package platform

import "testing"

func TestBootstrapSupportsPasswordDisabledSSO(t *testing.T) {
	a, err := BootstrapAdministrator(BootstrapInput{Username: "Admin", Role: "admin", SSOUser: true})
	if err != nil || a.UserID != "admin" || a.SourceType != "system" || a.SourceKey != "bootstrap" {
		t.Fatalf("invalid bootstrap assignment: %#v %v", a, err)
	}
	if _, err := BootstrapAdministrator(BootstrapInput{Username: "admin", Role: "admin"}); err == nil {
		t.Fatal("bootstrap without login method accepted")
	}
}
