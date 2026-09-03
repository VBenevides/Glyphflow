package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/config"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
	"github.com/nats-io/nats.go/jetstream"
	_ "modernc.org/sqlite"
)

type coverageStatusSink struct {
	runnerID, endpoint string
	capacity           *atomic.Int64
	running            func() int64
}

func (s *coverageStatusSink) SetRunnerID(value string)              { s.runnerID = value }
func (s *coverageStatusSink) SetNATSEndpoint(value string)          { s.endpoint = value }
func (s *coverageStatusSink) SetCapacitySource(value *atomic.Int64) { s.capacity = value }
func (s *coverageStatusSink) SetRunningSource(value func() int64)   { s.running = value }

func TestWorkerHelperFallbackBranches(t *testing.T) {
	if err := applyBootstrapTransportDefaults(nil); err != nil {
		t.Fatal(err)
	}
	if err := applyBootstrapTransportDefaults(&worker.Bootstrap{}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GLYPHFLOW_NATS_ENDPOINT", "")
	t.Setenv("RUNNER_NATS_URL", "nats://runner-env:4222")
	if got := resolveNATSEndpoint(nil, "nats://stored:4222"); got != "nats://runner-env:4222" {
		t.Fatalf("runner environment endpoint = %q", got)
	}
	t.Setenv("RUNNER_NATS_URL", "")
	if got := resolveNATSEndpoint(nil, ""); got != "" {
		t.Fatalf("empty NATS endpoint = %q", got)
	}

	t.Setenv("GLYPHFLOW_CONTROL_PLANE_URL", "")
	t.Setenv("RUNNER_CONTROL_PLANE_URL", "")
	if got := resolveControlPlaneEndpoint(nil); got != "" {
		t.Fatalf("empty control-plane endpoint = %q", got)
	}

	previous := setenv
	setenv = func(string, string) error { return errors.New("environment locked") }
	t.Cleanup(func() { setenv = previous })
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "")
	if err := applyBootstrapTransportDefaults(&worker.Bootstrap{AllowInsecureTransport: true}); err == nil || !strings.Contains(err.Error(), "set worker environment ENVIRONMENT") {
		t.Fatalf("transport default error = %v", err)
	}
}

func TestLogBufferFallbackBranches(t *testing.T) {
	buffer := NewLogBuffer(nil)
	buffer.SetParallelExecutions(4)
	if got := buffer.Snapshot(0).ParallelExecutions; got != 4 {
		t.Fatalf("initialized capacity = %d", got)
	}
	if written, err := buffer.Writer("unknown", nil).Write([]byte("ignored")); written != len("ignored") || err != nil {
		t.Fatalf("unknown stream write = %d, %v", written, err)
	}
	if got := buffer.Snapshot(0).Entries; len(got) != 0 {
		t.Fatalf("unknown stream added entries: %#v", got)
	}
}

func TestLoadPreviousBootIDRejectsWhitespace(t *testing.T) {
	store, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Put("worker.boot", "   "); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPreviousBootID(store); err == nil || !strings.Contains(err.Error(), "metadata is empty") {
		t.Fatalf("whitespace boot metadata error = %v", err)
	}
}

func TestRunnerHeartbeatHandlesPublishAndSigningErrors(t *testing.T) {
	var capacity atomic.Int64
	capacity.Store(2)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	workerHeartbeat(ctx, &queue.JetStream{}, "runner-1", "boot-1", key, &capacity, &stderr)
	if !strings.Contains(stderr.String(), "runner heartbeat:") {
		t.Fatalf("publish failure was not reported: %q", stderr.String())
	}
	workerHeartbeat(ctx, &queue.JetStream{}, "runner-1", "boot-1", protocol.SigningKey{}, &capacity, nil)
}

func TestRunWorkerProcessStopsBeforeNetworkListener(t *testing.T) {
	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		foundKey bool
		stored   protocol.SigningKey
		prepare  func(*worker.LocalStore) error
		natsURL  string
		want     string
	}{
		{name: "missing key is saved", natsURL: "http://invalid", want: "connect to NATS JetStream"},
		{name: "mismatched key is replaced", foundKey: true, stored: protocol.SigningKey{ID: "runner:old"}, natsURL: "http://invalid", want: "connect to NATS JetStream"},
		{name: "expired key is replaced", foundKey: true, stored: expiredWorkerKey(key), natsURL: "http://invalid", want: "connect to NATS JetStream"},
		{name: "previous boot is recovered", foundKey: true, stored: key, natsURL: "http://invalid", prepare: func(store *worker.LocalStore) error { return store.Put("worker.boot", "previous-boot") }, want: "connect to NATS JetStream"},
		{name: "TLS requires client files", foundKey: true, stored: key, natsURL: "tls://invalid", want: "connect to NATS JetStream"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			localStore, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
			if err != nil {
				t.Fatal(err)
			}
			defer localStore.Close()
			if test.prepare != nil {
				if err := test.prepare(localStore); err != nil {
					t.Fatal(err)
				}
			}
			cfg := config.Config{RunnerID: "runner-1", NATSURL: test.natsURL, DataDir: t.TempDir(), MaxOutputBytes: 1024}
			err = runWorkerProcess(context.Background(), io.Discard, io.Discard, nil, localStore, cfg, worker.RunnerConnection{}, test.stored, test.foundKey, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("runWorkerProcess() error = %v; want %q", err, test.want)
			}
		})
	}
}

func TestRunWorkerProcessReportsStoreFailures(t *testing.T) {
	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	closedStore, err := worker.OpenStore(t.TempDir() + "/closed.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := closedStore.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{RunnerID: "runner-1", NATSURL: "http://invalid", DataDir: t.TempDir(), MaxOutputBytes: 1024}
	if err := runWorkerProcess(context.Background(), io.Discard, io.Discard, nil, closedStore, cfg, worker.RunnerConnection{}, protocol.SigningKey{}, false, nil); err == nil || !strings.Contains(err.Error(), "save worker signing key") {
		t.Fatalf("closed store signing-key error = %v", err)
	}

	closedStore, err = worker.OpenStore(t.TempDir() + "/closed-boot.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := closedStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runWorkerProcess(context.Background(), io.Discard, io.Discard, nil, closedStore, cfg, worker.RunnerConnection{}, key, true, nil); err == nil || !strings.Contains(err.Error(), "read worker boot metadata") {
		t.Fatalf("closed store boot error = %v", err)
	}
}

func TestRunWorkerProcessReportsRecoveryFailure(t *testing.T) {
	localStore, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer localStore.Close()
	if err := localStore.PutOrder(worker.InboxOrder{
		OrderID:             "order-1",
		ExecutionAttemptID:  "attempt-1",
		RunID:               "run-1",
		RunnerID:            "runner-1",
		RunnerSessionID:     "session-1",
		ExecutorBootID:      "previous-boot",
		Envelope:            "malformed",
		LeaseToken:          "lease",
		LeaseNotAfter:       time.Now().UTC().Add(time.Hour),
		ExecutionSpecDigest: "digest",
		AttemptNumber:       1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := localStore.ClaimOrder("order-1", "previous-boot", 1); err != nil {
		t.Fatal(err)
	}
	if err := localStore.Put("worker.boot", "previous-boot"); err != nil {
		t.Fatal(err)
	}
	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{RunnerID: "runner-1", NATSURL: "nats://unused", DataDir: t.TempDir(), MaxOutputBytes: 1024}
	if err := runWorkerProcess(context.Background(), io.Discard, io.Discard, nil, localStore, cfg, worker.RunnerConnection{}, key, true, nil); err == nil || !strings.Contains(err.Error(), "recover durable worker events") {
		t.Fatalf("recovery error = %v", err)
	}
}

func TestRunWorkerReportsStartupAndConfigurationFailures(t *testing.T) {
	dataDir := t.TempDir()
	filePath := dataDir + "/not-a-directory"
	if err := os.WriteFile(filePath, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", filePath)
	if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "create worker data directory") {
		t.Fatalf("file DATA_DIR error = %v", err)
	}

	t.Setenv("DATA_DIR", dataDir)
	store, err := worker.OpenStore(dataDir + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put("runner.connection", "malformed"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "load worker connection") {
		t.Fatalf("malformed connection error = %v", err)
	}
}

func TestRunWorkerReportsBootstrapAndDataDirSetupFailures(t *testing.T) {
	previousLoader := loadEmbeddedBootstrap
	previousSetenv := setenv
	t.Cleanup(func() {
		loadEmbeddedBootstrap = previousLoader
		setenv = previousSetenv
	})

	loadEmbeddedBootstrap = func() (*worker.Bootstrap, error) { return nil, errors.New("bootstrap unreadable") }
	if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "load worker bootstrap") {
		t.Fatalf("bootstrap error = %v", err)
	}

	loadEmbeddedBootstrap = func() (*worker.Bootstrap, error) { return nil, nil }
	setenv = func(name, _ string) error {
		if name == "DATA_DIR" {
			return errors.New("environment locked")
		}
		return nil
	}
	t.Setenv("DATA_DIR", "")
	if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "set worker environment DATA_DIR") {
		t.Fatalf("DATA_DIR setup error = %v", err)
	}
}

func TestRunWorkerCoversStoreAndConfigFailures(t *testing.T) {
	t.Run("missing connection", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv("DATA_DIR", dataDir)
		if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "control-plane public key is unavailable") {
			t.Fatalf("missing connection error = %v", err)
		}
	})

	t.Run("store cannot open", func(t *testing.T) {
		dataDir := t.TempDir()
		if err := os.Mkdir(dataDir+"/runner.sqlite", 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATA_DIR", dataDir)
		if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "open worker store") {
			t.Fatalf("unopenable store error = %v", err)
		}
	})

	t.Run("signing key cannot load", func(t *testing.T) {
		dataDir := t.TempDir()
		store, err := worker.OpenStore(dataDir + "/runner.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Put("worker.signing_key", "malformed"); err != nil {
			store.Close()
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATA_DIR", dataDir)
		if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "load worker signing key") {
			t.Fatalf("signing key error = %v", err)
		}
	})

	t.Run("configuration fails after connection setup", func(t *testing.T) {
		dataDir := t.TempDir()
		controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		store, err := worker.OpenStore(dataDir + "/runner.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.SaveConnection(worker.RunnerConnection{RunnerID: "runner-1", NATSURL: "nats://127.0.0.1:1", MaxMessageBytes: 1024, ControlPublicKey: base64.RawStdEncoding.EncodeToString(controlKey.Public.PublicKey)}); err != nil {
			store.Close()
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATA_DIR", dataDir)
		t.Setenv("ENVIRONMENT", "development")
		t.Setenv("ALLOW_INSECURE_TRANSPORT", "true")
		t.Setenv("GLYPHFLOW_DATABASE", "invalid")
		t.Setenv("MAX_OUTPUT_BYTES", "")
		t.Setenv("ENABLE_PASSWORD_LOGIN", "not-a-boolean")
		if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "load worker configuration") {
			t.Fatalf("configuration error = %v", err)
		}
	})
}

func TestRunWorkerCoversBootstrapBranches(t *testing.T) {
	previousLoader := loadEmbeddedBootstrap
	t.Cleanup(func() { loadEmbeddedBootstrap = previousLoader })

	t.Run("rejects insecure enrollment endpoint", func(t *testing.T) {
		loadEmbeddedBootstrap = func() (*worker.Bootstrap, error) {
			return &worker.Bootstrap{RunnerID: "runner-1", ControlPlaneURL: "http://control.example"}, nil
		}
		t.Setenv("DATA_DIR", t.TempDir())
		t.Setenv("ENVIRONMENT", "production")
		if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "worker control-plane endpoint") {
			t.Fatalf("insecure enrollment endpoint error = %v", err)
		}
	})

	t.Run("reports failed enrollment", func(t *testing.T) {
		loadEmbeddedBootstrap = func() (*worker.Bootstrap, error) {
			return &worker.Bootstrap{Token: "enrollment-token", RunnerID: "runner-1", ControlPlaneURL: "https://control.example"}, nil
		}
		t.Setenv("DATA_DIR", t.TempDir())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := runWorker(ctx, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "runner enrollment failed") {
			t.Fatalf("enrollment error = %v", err)
		}
	})

	t.Run("refreshes control-plane key before config failure", func(t *testing.T) {
		controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		storedKey, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		newControlKey, err := protocol.GenerateSigningKey("new-control-plane", time.Now().UTC(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		loadEmbeddedBootstrap = func() (*worker.Bootstrap, error) {
			return &worker.Bootstrap{
				RunnerID:         "runner-1",
				ControlPublicKey: base64.RawStdEncoding.EncodeToString(newControlKey.Public.PublicKey),
				NATSURL:          "nats://embedded.example",
			}, nil
		}
		dataDir := t.TempDir()
		localStore, err := worker.OpenStore(dataDir + "/runner.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		connection := worker.RunnerConnection{RunnerID: "runner-1", NATSURL: "nats://stored.example", MaxMessageBytes: 1024, ControlPublicKey: base64.RawStdEncoding.EncodeToString(controlKey.Public.PublicKey)}
		if err := localStore.SaveConnection(connection); err != nil {
			localStore.Close()
			t.Fatal(err)
		}
		if err := localStore.SaveSigningKey(storedKey); err != nil {
			localStore.Close()
			t.Fatal(err)
		}
		if err := localStore.Close(); err != nil {
			t.Fatal(err)
		}
		t.Setenv("DATA_DIR", dataDir)
		t.Setenv("ENVIRONMENT", "development")
		t.Setenv("ALLOW_INSECURE_TRANSPORT", "true")
		t.Setenv("MAX_OUTPUT_BYTES", "")
		t.Setenv("ENABLE_PASSWORD_LOGIN", "not-a-boolean")
		if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "load worker configuration") {
			t.Fatalf("refreshed bootstrap configuration error = %v", err)
		}
		checkStore, err := worker.OpenStore(dataDir + "/runner.sqlite")
		if err != nil {
			t.Fatal(err)
		}
		defer checkStore.Close()
		got, found, err := checkStore.LoadConnection()
		if err != nil || !found || got.ControlPublicKey != base64.RawStdEncoding.EncodeToString(newControlKey.Public.PublicKey) {
			t.Fatalf("refreshed connection = %#v, found=%t, err=%v", got, found, err)
		}
	})
}

func TestRunWorkerReportsEnvironmentSetupFailures(t *testing.T) {
	controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	previousSetenv := setenv
	t.Cleanup(func() { setenv = previousSetenv })
	for _, name := range []string{"RUNNER_ID", "NATS_URL", "MAX_MESSAGE_BYTES", "MAX_OUTPUT_BYTES"} {
		t.Run(name, func(t *testing.T) {
			dataDir := t.TempDir()
			localStore, err := worker.OpenStore(dataDir + "/runner.sqlite")
			if err != nil {
				t.Fatal(err)
			}
			if err := localStore.SaveConnection(worker.RunnerConnection{RunnerID: "runner-1", NATSURL: "nats://stored.example", MaxMessageBytes: 1024, ControlPublicKey: base64.RawStdEncoding.EncodeToString(controlKey.Public.PublicKey)}); err != nil {
				localStore.Close()
				t.Fatal(err)
			}
			if err := localStore.Close(); err != nil {
				t.Fatal(err)
			}
			t.Setenv("DATA_DIR", dataDir)
			t.Setenv("MAX_OUTPUT_BYTES", "")
			setenv = func(variable, _ string) error {
				if variable == name {
					return errors.New("environment locked")
				}
				return nil
			}
			if err := runWorker(context.Background(), nil, nil, nil); err == nil || !strings.Contains(err.Error(), "set worker environment "+name) {
				t.Fatalf("%s setup error = %v", name, err)
			}
		})
	}
}

func TestRunWorkerProcessReportsBootWriteFailure(t *testing.T) {
	databasePath := t.TempDir() + "/runner.sqlite"
	store, err := worker.OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TRIGGER block_worker_boot BEFORE INSERT ON messages WHEN NEW.id = 'worker.boot' BEGIN SELECT RAISE(ABORT, 'boot writes disabled'); END`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = worker.OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{RunnerID: "runner-1", NATSURL: "nats://unused", DataDir: t.TempDir(), MaxOutputBytes: 1024}
	if err := runWorkerProcess(context.Background(), io.Discard, io.Discard, nil, store, cfg, worker.RunnerConnection{}, key, true, nil); err == nil || !strings.Contains(err.Error(), "save worker boot id") {
		t.Fatalf("boot write error = %v", err)
	}
}

func TestRunWorkerProcessInitializesRuntimeBeforeConsumerFailure(t *testing.T) {
	previousPlain := connectJetStreamPlain
	previousTLS := connectJetStreamTLS
	connectJetStreamPlain = func(context.Context, string) (*queue.JetStream, error) { return &queue.JetStream{}, nil }
	connectJetStreamTLS = func(context.Context, string, queue.TLSConfig) (*queue.JetStream, error) {
		return &queue.JetStream{}, nil
	}
	t.Cleanup(func() {
		connectJetStreamPlain = previousPlain
		connectJetStreamTLS = previousTLS
	})

	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		capacity int
		connect  string
	}{
		{name: "default capacity", connect: "nats://runtime"},
		{name: "configured capacity", capacity: 4, connect: "tls://runtime"},
	} {
		t.Run(test.name, func(t *testing.T) {
			localStore, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
			if err != nil {
				t.Fatal(err)
			}
			defer localStore.Close()
			status := &coverageStatusSink{}
			cfg := config.Config{RunnerID: "runner-1", NATSURL: test.connect, DataDir: t.TempDir(), MaxOutputBytes: 1024}
			err = runWorkerProcess(context.Background(), io.Discard, io.Discard, status, localStore, cfg, worker.RunnerConnection{Capacity: test.capacity}, key, true, make([]byte, ed25519.PublicKeySize))
			if err == nil || !strings.Contains(err.Error(), "create order consumer") {
				t.Fatalf("consumer setup error = %v", err)
			}
			wantCapacity := int64(test.capacity)
			if wantCapacity == 0 {
				wantCapacity = store.DefaultRunnerCapacity
			}
			if status.capacity == nil || status.capacity.Load() != wantCapacity || status.running == nil {
				t.Fatalf("status sources = %#v, capacity=%v", status, status.capacity)
			}
		})
	}
}

func TestRunWorkerProcessRunsBackgroundLoopsWithoutListener(t *testing.T) {
	previousPlain := connectJetStreamPlain
	previousConsumer := createJetStreamConsumer
	connectJetStreamPlain = func(context.Context, string) (*queue.JetStream, error) { return &queue.JetStream{}, nil }
	createJetStreamConsumer = func(*queue.JetStream, context.Context, string, string, int) (jetstream.Consumer, error) {
		return nil, nil
	}
	t.Cleanup(func() {
		connectJetStreamPlain = previousPlain
		createJetStreamConsumer = previousConsumer
	})

	localStore, err := worker.OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer localStore.Close()
	key, err := protocol.GenerateSigningKey("runner:runner-1", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	var stdout bytes.Buffer
	cfg := config.Config{RunnerID: "runner-1", NATSURL: "nats://runtime", DataDir: t.TempDir(), MaxOutputBytes: 1024}
	if err := runWorkerProcess(ctx, &stdout, io.Discard, nil, localStore, cfg, worker.RunnerConnection{}, key, true, make([]byte, ed25519.PublicKeySize)); err != nil {
		t.Fatalf("runtime shutdown error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Glyphflow worker v") {
		t.Fatalf("worker banner = %q", stdout.String())
	}
}

func TestRunWorkerPublishesStatusBeforeNATSFailure(t *testing.T) {
	dataDir := t.TempDir()
	controlKey, err := protocol.GenerateSigningKey("control-plane", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	store, err := worker.OpenStore(dataDir + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveConnection(worker.RunnerConnection{RunnerID: "runner-1", NATSURL: "nats://127.0.0.1:1", MaxMessageBytes: 2048, Capacity: 3, ControlPublicKey: base64.RawStdEncoding.EncodeToString(ed25519.PublicKey(controlKey.Public.PublicKey))}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("MAX_OUTPUT_BYTES", "")
	t.Setenv("GLYPHFLOW_DATABASE", "sqlite")
	t.Setenv("DATABASE_URL", dataDir+"/controlplane.sqlite")
	status := &coverageStatusSink{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runWorker(ctx, nil, nil, status); err == nil || !strings.Contains(err.Error(), "connect to NATS JetStream") {
		t.Fatalf("NATS connection error = %v", err)
	}
	if status.runnerID != "runner-1" || status.endpoint != "nats://127.0.0.1:1" {
		t.Fatalf("status = %#v", status)
	}
}

func expiredWorkerKey(key protocol.SigningKey) protocol.SigningKey {
	key.Public.NotAfter = time.Now().UTC().Add(-time.Hour)
	return key
}
