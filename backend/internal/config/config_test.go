package config

import "testing"

func TestValidateControlPlane(t *testing.T) {
	config := Config{
		Role:              ControlPlane,
		DatabaseURL:       "postgres://user:pass@localhost/db",
		NATSURL:           "nats://localhost:4222",
		AccessTokenSecret: "01234567890123456789012345678901",
		WebOrigin:         "https://console.example",
		DataDir:           "/var/lib/glyphflow",
		MaxMessageBytes:   1 << 20,
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("valid control-plane config rejected: %v", err)
	}
}

func TestValidateWorkerRejectsOversizedOutput(t *testing.T) {
	config := Config{
		Role:            Worker,
		NATSURL:         "nats://localhost:4222",
		DataDir:         "/var/lib/glyphflow",
		RunnerID:        "worker-1",
		MaxMessageBytes: 1024,
		MaxOutputBytes:  2048,
	}
	if err := config.Validate(); err == nil {
		t.Fatal("worker config with oversized output limit was accepted")
	}
}

func TestValidateControlPlaneRequiresPasswordPepper(t *testing.T) {
	config := Config{
		Role:                 ControlPlane,
		DatabaseURL:          "postgres://user:pass@localhost/db",
		NATSURL:              "nats://localhost:4222",
		AccessTokenSecret:    "01234567890123456789012345678901",
		PasswordLoginEnabled: true,
		WebOrigin:            "https://console.example",
		DataDir:              "/var/lib/glyphflow",
		MaxMessageBytes:      1 << 20,
	}
	if err := config.Validate(); err == nil {
		t.Fatal("missing password pepper accepted")
	}
	config.PasswordPepper = "0123456789012345"
	if err := config.Validate(); err != nil {
		t.Fatalf("password pepper rejected: %v", err)
	}
}
