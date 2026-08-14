package platform

import "testing"

func TestBootstrapSupportsPasswordDisabledSSO(t *testing.T) {
	a, err := BootstrapAdministrator(BootstrapInput{Username: "Admin@example.com", Role: "admin", SSOUser: true})
	if err != nil || a.UserID != "admin@example.com" || a.SourceType != "system" || a.SourceKey != "bootstrap" {
		t.Fatalf("invalid bootstrap assignment: %#v %v", a, err)
	}
	if _, err := BootstrapAdministrator(BootstrapInput{Username: "admin@example.com", Role: "admin"}); err == nil {
		t.Fatal("bootstrap without login method accepted")
	}
}
