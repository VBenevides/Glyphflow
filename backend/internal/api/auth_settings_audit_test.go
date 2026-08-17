package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticationSettingsAuditContainsBeforeAndAfter(t *testing.T) {
	auth, err := NewAuthService("01234567890123456789012345678901", true, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.AddRole("user"); err != nil {
		t.Fatal(err)
	}
	auth.SetDefaultRole("user")
	user, err := auth.Register("admin@example.com", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	oidc := NewOIDCService()
	if err := oidc.AddProvider(OIDCProvider{Key: "corp", Issuer: "https://issuer.example", Callback: "https://app.example/callback", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	audit := NewAuditQueryService()
	server := Server{AuthService: auth, Auth: func(*http.Request) (Claims, bool) { return Claims{UserID: user.ID}, true }, Permissions: func(Claims) map[string]bool { return map[string]bool{"auth.settings.manage": true} }, AuditQuery: audit}
	server.AuthAdmin = &AuthAdminService{Auth: auth, OIDC: oidc}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/settings", bytes.NewBufferString(`{"enabled":false,"registration":false,"default_role_id":"system-user"}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("settings update: %d", response.Code)
	}
	if len(audit.events) != 1 || audit.events[0].Action != http.MethodPost {
		t.Fatalf("settings audit count/action: %#v", audit.events)
	}
	detailed := &audit.events[0]
	if detailed.Before["passwordLoginEnabled"] != true || detailed.After["passwordLoginEnabled"] != false {
		t.Fatalf("settings before/after missing: %#v", audit.events)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/settings", bytes.NewBufferString(`{"enabled":false,"registration":false,"default_role_id":"missing"}`))
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || audit.events[len(audit.events)-1].Result != "failure" {
		t.Fatalf("failed settings audit: %d %#v", response.Code, audit.events[len(audit.events)-1])
	}
}
