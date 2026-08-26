package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

func TestNeedsRunnerEnrollmentForUnenrolledStore(t *testing.T) {
	bootstrap := &worker.Bootstrap{RunnerID: "runner-1"}
	validKey, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	expiredKey := validKey
	expiredKey.Public.NotAfter = time.Now().UTC().Add(-time.Hour)
	for _, test := range []struct {
		name                      string
		connectionFound, keyFound bool
		key                       protocol.SigningKey
		want                      bool
	}{
		{name: "new store", want: true},
		{name: "stored connection without key", connectionFound: true, want: true},
		{name: "changed key identity", connectionFound: true, keyFound: true, key: protocol.SigningKey{ID: "runner:old"}, want: true},
		{name: "expired key", connectionFound: true, keyFound: true, key: expiredKey, want: true},
		{name: "enrolled store", connectionFound: true, keyFound: true, key: validKey, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := needsRunnerEnrollment(bootstrap, test.connectionFound, test.keyFound, test.key); got != test.want {
				t.Fatalf("needsRunnerEnrollment() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestResolveNATSEndpointPriority(t *testing.T) {
	bootstrap := &worker.Bootstrap{NATSURL: "nats://embedded:4222"}
	t.Setenv("GLYPHFLOW_NATS_ENDPOINT", "nats://environment:4222")
	t.Setenv("RUNNER_NATS_URL", "")
	if got := resolveNATSEndpoint(bootstrap, "nats://server:4222"); got != "nats://environment:4222" {
		t.Fatalf("environment endpoint = %q", got)
	}
	t.Setenv("GLYPHFLOW_NATS_ENDPOINT", "")
	if got := resolveNATSEndpoint(bootstrap, "nats://server:4222"); got != "nats://embedded:4222" {
		t.Fatalf("embedded endpoint = %q", got)
	}
	if got := resolveNATSEndpoint(nil, "nats://server:4222"); got != "nats://server:4222" {
		t.Fatalf("server endpoint = %q", got)
	}
}

func TestResolveControlPlaneEndpointPriority(t *testing.T) {
	bootstrap := &worker.Bootstrap{ControlPlaneURL: "http://embedded:8080"}
	t.Setenv("GLYPHFLOW_CONTROL_PLANE_URL", "http://environment:8080")
	t.Setenv("RUNNER_CONTROL_PLANE_URL", "")
	if got := resolveControlPlaneEndpoint(bootstrap); got != "http://environment:8080" {
		t.Fatalf("environment endpoint = %q", got)
	}
	t.Setenv("GLYPHFLOW_CONTROL_PLANE_URL", "")
	t.Setenv("RUNNER_CONTROL_PLANE_URL", "http://runner-environment:8080")
	if got := resolveControlPlaneEndpoint(bootstrap); got != "http://runner-environment:8080" {
		t.Fatalf("runner environment endpoint = %q", got)
	}
	t.Setenv("RUNNER_CONTROL_PLANE_URL", "")
	if got := resolveControlPlaneEndpoint(bootstrap); got != "http://embedded:8080" {
		t.Fatalf("embedded endpoint = %q", got)
	}
}

func TestLoadPreviousBootID(t *testing.T) {
	for _, test := range []struct {
		name    string
		prepare func(*worker.LocalStore) error
		want    string
		wantErr string
	}{
		{name: "missing"},
		{
			name: "valid",
			prepare: func(store *worker.LocalStore) error {
				return store.Put("worker.boot", "boot-1")
			},
			want: "boot-1",
		},
		{
			name: "malformed",
			prepare: func(store *worker.LocalStore) error {
				return store.Put("worker.boot", map[string]string{"boot_id": "boot-1"})
			},
			wantErr: "decode worker boot metadata",
		},
		{
			name: "read failure",
			prepare: func(store *worker.LocalStore) error {
				return store.Close()
			},
			wantErr: "read worker boot metadata",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
			if err != nil {
				t.Fatal(err)
			}
			if test.prepare != nil {
				if err := test.prepare(store); err != nil && test.name != "read failure" {
					t.Fatal(err)
				}
			} else {
				defer store.Close()
			}
			got, err := loadPreviousBootID(store)
			if test.wantErr == "" {
				if err != nil || got != test.want {
					t.Fatalf("loadPreviousBootID() = %q, %v; want %q, nil", got, err, test.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("loadPreviousBootID() error = %v; want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestSetWorkerEnvReportsFailure(t *testing.T) {
	previous := setenv
	setenv = func(string, string) error { return errors.New("environment locked") }
	t.Cleanup(func() { setenv = previous })

	if err := setWorkerEnv("NATS_URL", "nats://example:4222"); err == nil || !strings.Contains(err.Error(), "set worker environment NATS_URL") {
		t.Fatalf("setWorkerEnv() error = %v", err)
	}
}
