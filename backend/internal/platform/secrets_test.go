package platform

import (
	"testing"
	"time"
)

func TestSecretResolverKeepsReferencesAndValuesSeparate(t *testing.T) {
	resolver := NewSecretResolver()
	if err := resolver.Add("secret://oidc/client", "private-value", 1, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	value, metadata, err := resolver.Resolve("secret://oidc/client", time.Now())
	if err != nil || value != "private-value" || metadata.Version != 1 {
		t.Fatalf("secret resolution failed: %q %#v %v", value, metadata, err)
	}
	resolver.Revoke("secret://oidc/client")
	if _, _, err := resolver.Resolve("secret://oidc/client", time.Now()); err == nil {
		t.Fatal("revoked secret resolved")
	}
}
