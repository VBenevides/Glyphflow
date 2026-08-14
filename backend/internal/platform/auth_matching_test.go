package platform

import "testing"

func TestSSOMatchingUsesProviderAndSubjectOnly(t *testing.T) {
	identities := []SSOIdentity{{UserID: "u1", Provider: "corp", Subject: "sub-1"}}
	if user, ok := MatchSSOIdentity(identities, "corp", "sub-1"); !ok || user != "u1" {
		t.Fatal("exact identity did not match")
	}
	if _, ok := MatchSSOIdentity(identities, "other", "sub-1"); ok {
		t.Fatal("cross-provider identity matched")
	}
	if err := CanLinkSSOIdentity("u1", "u2"); err == nil {
		t.Fatal("unauthenticated account link accepted")
	}
}
