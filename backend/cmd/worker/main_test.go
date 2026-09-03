package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

func TestRunnerHeartbeatPayloadIncludesResourceMetrics(t *testing.T) {
	raw := runnerHeartbeatPayload("runner-1", "boot-1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 4, &runnerResourceMetrics{CPUPercent: 12.5, MemoryPercent: 37.5, MemoryUsedBytes: 100, MemoryTotalBytes: 200})
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]float64{"cpu_percent": 12.5, "memory_percent": 37.5, "memory_used_bytes": 100, "memory_total_bytes": 200} {
		if payload[key] != want {
			t.Fatalf("payload[%q] = %v, want %v", key, payload[key], want)
		}
	}
}

func TestSampleRunnerResourcesReadsHostMemory(t *testing.T) {
	metrics := sampleRunnerResources()
	if metrics == nil || metrics.MemoryTotalBytes <= 0 || metrics.MemoryUsedBytes < 0 || metrics.MemoryPercent < 0 || metrics.MemoryPercent > 100 || metrics.CPUPercent < 0 || metrics.CPUPercent > 100 {
		t.Fatalf("sampled resources = %#v", metrics)
	}
}

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

func TestApplyBootstrapTransportDefaults(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "")
	if err := applyBootstrapTransportDefaults(&worker.Bootstrap{AllowInsecureTransport: true}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ENVIRONMENT") != "development" || os.Getenv("ALLOW_INSECURE_TRANSPORT") != "true" {
		t.Fatalf("transport defaults = %q/%q", os.Getenv("ENVIRONMENT"), os.Getenv("ALLOW_INSECURE_TRANSPORT"))
	}
	if err := validateWorkerControlPlaneEndpoint("http://control.example"); err != nil {
		t.Fatalf("development HTTP endpoint rejected: %v", err)
	}

	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "false")
	if err := applyBootstrapTransportDefaults(&worker.Bootstrap{AllowInsecureTransport: true}); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("ENVIRONMENT") != "production" || os.Getenv("ALLOW_INSECURE_TRANSPORT") != "false" {
		t.Fatalf("explicit transport settings changed = %q/%q", os.Getenv("ENVIRONMENT"), os.Getenv("ALLOW_INSECURE_TRANSPORT"))
	}
}

func TestValidateWorkerControlPlaneEndpoint(t *testing.T) {
	for _, test := range []struct {
		name, environment, allow, endpoint, wantErr string
	}{
		{name: "secure production", environment: "production", allow: "false", endpoint: "https://control.example"},
		{name: "insecure development opt in", environment: "development", allow: "true", endpoint: "http://control.example"},
		{name: "production rejects HTTP", environment: "production", allow: "false", endpoint: "http://control.example", wantErr: "HTTPS"},
		{name: "development requires opt in", environment: "development", allow: "false", endpoint: "http://control.example", wantErr: "HTTPS"},
		{name: "missing host", environment: "production", allow: "false", endpoint: "https://", wantErr: "URL"},
		{name: "userinfo", environment: "production", allow: "false", endpoint: "https://user@control.example", wantErr: "URL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", test.environment)
			t.Setenv("ALLOW_INSECURE_TRANSPORT", test.allow)
			err := validateWorkerControlPlaneEndpoint(test.endpoint)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateWorkerControlPlaneEndpoint() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateWorkerControlPlaneEndpoint() error = %v; want %q", err, test.wantErr)
			}
		})
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
			assertPreviousBootID(t, test.prepare, test.name, test.want, test.wantErr)
		})
	}
}

func assertPreviousBootID(t *testing.T, prepare func(*worker.LocalStore) error, name, want, wantErr string) {
	t.Helper()
	store, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if prepare != nil {
		if err := prepare(store); err != nil && name != "read failure" {
			t.Fatal(err)
		}
	} else {
		defer store.Close()
	}
	got, err := loadPreviousBootID(store)
	if wantErr == "" {
		if err != nil || got != want {
			t.Fatalf("loadPreviousBootID() = %q, %v; want %q, nil", got, err, want)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("loadPreviousBootID() error = %v; want substring %q", err, wantErr)
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

func TestRunWorkerReturnsOnStartupCancellationAndClosesStore(t *testing.T) {
	dataDir := t.TempDir()
	controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	localStore, err := worker.OpenStore(dataDir + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := localStore.SaveConnection(worker.RunnerConnection{
		RunnerID:         "runner-1",
		NATSURL:          "nats://127.0.0.1:1",
		MaxMessageBytes:  1 << 20,
		ControlPublicKey: base64.RawStdEncoding.EncodeToString(ed25519.PublicKey(controlKey.Public.PublicKey)),
	}); err != nil {
		localStore.Close()
		t.Fatal(err)
	}
	if err := localStore.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "true")

	closed := false
	previousClose := closeWorkerStore
	closeWorkerStore = func(store *worker.LocalStore) {
		previousClose(store)
		closed = true
	}
	t.Cleanup(func() { closeWorkerStore = previousClose })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runWorker(ctx, io.Discard, io.Discard, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("runWorker cancellation = %v", err)
	}
	if !closed {
		t.Fatal("runWorker did not close the local store")
	}
}
