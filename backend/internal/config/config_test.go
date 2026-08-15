package config

import "testing"

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
}

func TestFromEnvRejectsMalformedBoolean(t *testing.T) {
	t.Setenv("ENABLE_PASSWORD_LOGIN", "sometimes")
	if _, err := FromEnv(ControlPlane); err == nil {
		t.Fatal("malformed password-login boolean was accepted")
	}
}
