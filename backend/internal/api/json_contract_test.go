package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFrontendAuthenticationJSONContract(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user", "runs.read", "tasks.read"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("user@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	profileData, err := auth.Profile(Claims{UserID: user.ID})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := json.Marshal(profileData)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(profile), `"permissions":["runs.read","tasks.read"]`) {
		t.Fatalf("permissions are not a sorted JSON array: %s", profile)
	}
	tokens := AuthTokens{AccessToken: "a", RefreshToken: "r", SessionID: "s"}
	tokenJSON, _ := json.Marshal(tokens)
	if string(tokenJSON) != `{"access_token":"a","refresh_token":"r","session_id":"s"}` {
		t.Fatalf("token contract: %s", tokenJSON)
	}
	providerJSON, _ := json.Marshal(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", ClientID: "client", Callback: "https://app.example/callback", Enabled: true})
	if !strings.Contains(string(providerJSON), `"clientId":"client"`) {
		t.Fatalf("provider contract: %s", providerJSON)
	}
}
