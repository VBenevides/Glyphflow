package config

import (
	"encoding/base64"
	"strings"
	"testing"

	"crypto/ed25519"
)

func coverageControlPlaneConfig() Config {
	return Config{
		Role:                          ControlPlane,
		DatabaseMode:                  "postgresql",
		DatabaseURL:                   "postgres://user:pass@db/glyphflow?sslmode=verify-full",
		NATSMode:                      "remote",
		NATSURL:                       "tls://nats.example:4222",
		NATSCertFile:                  "/cert",
		NATSKeyFile:                   "/key",
		NATSCAFile:                    "/ca",
		AccessTokenSecret:             "01234567890123456789012345678901",
		ControlPlaneSigningPrivateKey: base64.RawStdEncoding.EncodeToString(make([]byte, ed25519.PrivateKeySize)),
		PasswordPepper:                "0123456789012345",
		WebOrigin:                     "https://console.example",
		Environment:                   "staging",
		DataDir:                       "/var/lib/glyphflow",
		MaxMessageBytes:               1024,
		LogMonthsKeep:                 3,
		AuditMonthsKeep:               12,
		RunnerMetricsMonthsKeep:       3,
		DatabaseStorageCapacityBytes:  1 << 30,
	}
}

func assertConfigError(t *testing.T, config Config, contains string) {
	t.Helper()
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), contains) {
		t.Fatalf("Validate() = %v, want %q", err, contains)
	}
}

func TestConfigValidationEdges(t *testing.T) {
	config := coverageControlPlaneConfig()
	config.Role = "unknown"
	assertConfigError(t, config, "role")

	config = coverageControlPlaneConfig()
	config.SystemAdminEmails = []string{"not-an-email"}
	assertConfigError(t, config, "system admin email")

	config = coverageControlPlaneConfig()
	config.DatabaseMode, config.DatabaseURL = "", ""
	assertConfigError(t, config, "GLYPHFLOW_DATABASE")

	config = coverageControlPlaneConfig()
	config.NATSMode, config.NATSURL = "", ""
	assertConfigError(t, config, "NATS_URL")

	config = coverageControlPlaneConfig()
	config.NATSURL = "http://nats.example:4222"
	assertConfigError(t, config, "NATS_URL")

	config = coverageControlPlaneConfig()
	config.DataDir = "relative"
	assertConfigError(t, config, "DATA_DIR")

	config = coverageControlPlaneConfig()
	config.MaxMessageBytes = 0
	assertConfigError(t, config, "MAX_MESSAGE_BYTES")

	config = coverageControlPlaneConfig()
	config.LogMonthsKeep = 0
	assertConfigError(t, config, "LOG_MONTHS_KEEP")

	config = coverageControlPlaneConfig()
	config.DatabaseStorageCapacityBytes = -1
	assertConfigError(t, config, "DATABASE_STORAGE_CAPACITY_BYTES")

	config = coverageControlPlaneConfig()
	config.AccessTokenSecret = "short"
	assertConfigError(t, config, "ACCESS_TOKEN_SECRET")

	config = coverageControlPlaneConfig()
	config.PasswordLoginEnabled, config.PasswordPepper = true, "short"
	assertConfigError(t, config, "PASSWORD_PEPPER")

	config = coverageControlPlaneConfig()
	config.WebOrigin = "ftp://console.example"
	assertConfigError(t, config, "WEB_ORIGIN")

	config = coverageControlPlaneConfig()
	config.ControlPlaneSigningPrivateKey = "invalid"
	assertConfigError(t, config, "CONTROL_PLANE_SIGNING_PRIVATE_KEY")

	config = coverageControlPlaneConfig()
	config.NATSCertFile, config.NATSKeyFile, config.NATSCAFile = "", "", ""
	config.Environment = "production"
	assertConfigError(t, config, "production requires NATS")

	config = coverageControlPlaneConfig()
	config.DatabaseURL = "postgres://user:pass@db/glyphflow?sslmode=disable"
	assertConfigError(t, config, "sslmode=verify-full")

	config = coverageControlPlaneConfig()
	config.DatabaseMode, config.DatabaseURL = "sqlite", "file://db.sqlite"
	config.Environment = "development"
	config.AllowInsecureTransport = true
	config.ControlPlaneSigningPrivateKey = ""
	config.NATSMode, config.NATSURL = "embedded", "nats://127.0.0.1:4222"
	if err := config.Validate(); err != nil {
		t.Fatalf("valid development SQLite config rejected: %v", err)
	}
}

func TestWorkerConfigValidationEdges(t *testing.T) {
	config := Config{Role: Worker, NATSURL: "tls://nats.example:4222", DataDir: "/var/lib/glyphflow", RunnerID: "runner-1", MaxMessageBytes: 1024, MaxOutputBytes: 1024, Environment: "production"}
	config.RunnerID = ""
	assertConfigError(t, config, "RUNNER_ID")

	config.RunnerID = "runner-1"
	config.MaxOutputBytes = 0
	assertConfigError(t, config, "MAX_OUTPUT_BYTES")
}
