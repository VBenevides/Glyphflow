package config

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateControlPlane(t *testing.T) {
	config := Config{
		Role:                   ControlPlane,
		DatabaseURL:            "postgres://user:pass@localhost/db",
		NATSURL:                "nats://localhost:4222",
		AccessTokenSecret:      "01234567890123456789012345678901",
		WebOrigin:              "https://console.example",
		DataDir:                "/var/lib/glyphflow",
		MaxMessageBytes:        1 << 20,
		Environment:            "development",
		AllowInsecureTransport: true,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("valid control-plane config rejected: %v", err)
	}
}

func TestValidateWorkerRejectsOversizedOutput(t *testing.T) {
	config := Config{
		Role:                   Worker,
		NATSURL:                "nats://localhost:4222",
		DataDir:                "/var/lib/glyphflow",
		RunnerID:               "worker-1",
		MaxMessageBytes:        1024,
		MaxOutputBytes:         2048,
		Environment:            "development",
		AllowInsecureTransport: true,
	}
	if err := config.Validate(); err == nil {
		t.Fatal("worker config with oversized output limit was accepted")
	}
}

func TestValidateControlPlaneRequiresPasswordPepper(t *testing.T) {
	config := Config{
		Role:                   ControlPlane,
		DatabaseURL:            "postgres://user:pass@localhost/db",
		NATSURL:                "nats://localhost:4222",
		AccessTokenSecret:      "01234567890123456789012345678901",
		PasswordLoginEnabled:   true,
		WebOrigin:              "https://console.example",
		DataDir:                "/var/lib/glyphflow",
		MaxMessageBytes:        1 << 20,
		Environment:            "development",
		AllowInsecureTransport: true,
	}
	if err := config.Validate(); err == nil {
		t.Fatal("missing password pepper accepted")
	}
	config.PasswordPepper = "0123456789012345"
	if err := config.Validate(); err != nil {
		t.Fatalf("password pepper rejected: %v", err)
	}
}

func TestFromEnvParsesSystemAdminEmails(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	t.Setenv("NATS_URL", "nats://localhost:4222")
	t.Setenv("ACCESS_TOKEN_SECRET", "01234567890123456789012345678901")
	t.Setenv("PASSWORD_PEPPER", "0123456789012345")
	t.Setenv("WEB_ORIGIN", "https://console.example")
	t.Setenv("CORS_ORIGIN", "address1,address2, address3")
	t.Setenv("CSRF_ORIGINS", "https://console.example, http://localhost:5173")
	t.Setenv("DATA_DIR", "/var/lib/glyphflow")
	t.Setenv("MAX_MESSAGE_BYTES", "1024")
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("GLYPHFLOW_SYSTEM_ADMINS", "one@example.com, two@example.com;one@example.com")

	config, err := FromEnv(ControlPlane)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.SystemAdminEmails) != 2 || config.SystemAdminEmails[0] != "one@example.com" || config.SystemAdminEmails[1] != "two@example.com" {
		t.Fatalf("system admins = %#v", config.SystemAdminEmails)
	}
	if got, want := strings.Join(config.CORSOrigins, ","), "address1,address2,address3"; got != want {
		t.Fatalf("CORS origins = %q", got)
	}
	if got, want := strings.Join(config.CSRFOrigins, ","), "https://console.example,http://localhost:5173"; got != want {
		t.Fatalf("CSRF origins = %q", got)
	}
}

func TestFromEnvRejectsMalformedBoolean(t *testing.T) {
	t.Setenv("ENABLE_PASSWORD_LOGIN", "sometimes")
	if _, err := FromEnv(ControlPlane); err == nil {
		t.Fatal("malformed password-login boolean was accepted")
	}
}

func TestDatabaseSSLMode(t *testing.T) {
	if got := databaseSSLMode("postgres://user:pass@db/glyphflow?sslmode=verify-full"); got != "verify-full" {
		t.Fatalf("databaseSSLMode() = %q", got)
	}
	if got := databaseSSLMode("postgres://user:pass@db/glyphflow?sslmode=disable"); got != "disable" {
		t.Fatalf("databaseSSLMode() = %q", got)
	}
	if got := databaseSSLMode("not a URL"); got != "" {
		t.Fatalf("databaseSSLMode(invalid) = %q", got)
	}
}

func TestValidateControlPlaneRequiresPostgresTLS(t *testing.T) {
	config := Config{
		Role:                          ControlPlane,
		DatabaseURL:                   "postgres://user:pass@db/glyphflow?sslmode=disable",
		NATSURL:                       "tls://nats:4222",
		NATSCertFile:                  "/run/secrets/nats-cert",
		NATSKeyFile:                   "/run/secrets/nats-key",
		NATSCAFile:                    "/run/secrets/nats-ca",
		AccessTokenSecret:             "01234567890123456789012345678901",
		ControlPlaneSigningPrivateKey: base64.RawStdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize)),
		PasswordPepper:                "0123456789012345",
		WebOrigin:                     "https://console.example",
		DataDir:                       "/var/lib/glyphflow",
		MaxMessageBytes:               1 << 20,
		Environment:                   "staging",
	}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "sslmode=verify-full") {
		t.Fatalf("plaintext database URL error = %v", err)
	}
	config.DatabaseURL = "postgres://user:pass@db/glyphflow?sslmode=verify-full"
	if err := config.Validate(); err != nil {
		t.Fatalf("TLS database URL rejected: %v", err)
	}
}
