package config

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
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
		LogMonthsKeep:          3,
		AuditMonthsKeep:        12,
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
		LogMonthsKeep:          3,
		AuditMonthsKeep:        12,
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
	t.Setenv("LOG_MONTHS_KEEP", "3")
	t.Setenv("AUDIT_MONTHS_KEEP", "12")

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
	if config.LogMonthsKeep != 3 || config.AuditMonthsKeep != 12 {
		t.Fatalf("retention months = %d/%d", config.LogMonthsKeep, config.AuditMonthsKeep)
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
		SecretEncryptionKeyFile:       "/run/secrets/encryption-key",
		PasswordPepper:                "0123456789012345",
		WebOrigin:                     "https://console.example",
		DataDir:                       "/var/lib/glyphflow",
		LogMonthsKeep:                 3,
		AuditMonthsKeep:               12,
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

func TestLoadInstallationEncryptionKey(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, installationEncryptionKeySize)
	path := filepath.Join(t.TempDir(), "encryption.key")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadInstallationEncryptionKey("production", path, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, key) {
		t.Fatalf("loaded key = %x, want %x", loaded, key)
	}

	cases := []struct {
		name string
		data string
		mode os.FileMode
	}{
		{name: "missing", mode: 0o600},
		{name: "unreadable", data: base64.StdEncoding.EncodeToString(key), mode: 0o000},
		{name: "broad permissions", data: base64.StdEncoding.EncodeToString(key), mode: 0o644},
		{name: "malformed", data: "not-base64", mode: 0o600},
		{name: "wrong length", data: base64.StdEncoding.EncodeToString(key[:16]), mode: 0o600},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			casePath := filepath.Join(t.TempDir(), "encryption.key")
			if test.name != "missing" {
				if err := os.WriteFile(casePath, []byte(test.data), test.mode); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := loadInstallationEncryptionKey("production", casePath, "fallback"); err == nil {
				t.Fatal("invalid installation key file was accepted")
			}
		})
	}
}

func TestFromEnvLoadsInstallationKeyOnceAndKeepsDevelopmentFallback(t *testing.T) {
	fallback := "01234567890123456789012345678901"
	t.Setenv("DATABASE_URL", "postgres://user:pass@db/glyphflow?sslmode=verify-full")
	t.Setenv("NATS_URL", "tls://nats:4222")
	t.Setenv("NATS_CERT_FILE", "/run/secrets/nats-cert")
	t.Setenv("NATS_KEY_FILE", "/run/secrets/nats-key")
	t.Setenv("NATS_CA_FILE", "/run/secrets/nats-ca")
	t.Setenv("ACCESS_TOKEN_SECRET", fallback)
	t.Setenv("CONTROL_PLANE_SIGNING_PRIVATE_KEY", base64.RawStdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize)))
	t.Setenv("PASSWORD_PEPPER", "0123456789012345")
	t.Setenv("WEB_ORIGIN", "https://console.example")
	t.Setenv("DATA_DIR", "/var/lib/glyphflow")
	t.Setenv("MAX_MESSAGE_BYTES", "1024")
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "false")
	key := bytes.Repeat([]byte{0x7f}, installationEncryptionKeySize)
	path := filepath.Join(t.TempDir(), "encryption.key")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SECRET_ENCRYPTION_KEY_FILE", path)
	config, err := FromEnv(ControlPlane)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(config.InstallationEncryptionKey, key) {
		t.Fatalf("installation key = %x, want %x", config.InstallationEncryptionKey, key)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(config.InstallationEncryptionKey, key) {
		t.Fatal("loaded installation key changed after file deletion")
	}
	if _, err := FromEnv(ControlPlane); err == nil {
		t.Fatal("restart accepted a deleted installation key file")
	}

	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("SECRET_ENCRYPTION_KEY_FILE", "")
	development, err := FromEnv(ControlPlane)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(development.InstallationEncryptionKey, []byte(fallback)) {
		t.Fatalf("development fallback = %q, want access-token secret", development.InstallationEncryptionKey)
	}
}
