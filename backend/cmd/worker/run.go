package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VBenevides/Glyphflow/backend"
	"github.com/VBenevides/Glyphflow/backend/internal/config"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

func runWorker(ctx context.Context, stdout, stderr io.Writer, status StatusSink) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	bootstrap, err := worker.LoadEmbeddedBootstrap()
	if err != nil {
		return fmt.Errorf("load worker bootstrap: %w", err)
	}
	if os.Getenv("DATA_DIR") == "" {
		dataDir := worker.DefaultDataDir()
		if bootstrap != nil {
			dataDir = filepath.Join(dataDir, bootstrap.RunnerID)
		}
		if err := setWorkerEnv("DATA_DIR", dataDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(os.Getenv("DATA_DIR"), 0o700); err != nil {
		return fmt.Errorf("create worker data directory: %w", err)
	}
	localStore, err := worker.OpenStore(filepath.Join(os.Getenv("DATA_DIR"), "runner.sqlite"))
	if err != nil {
		return fmt.Errorf("open worker store: %w", err)
	}
	defer func() { _ = localStore.Close() }()
	connection, found, err := localStore.LoadConnection()
	if err != nil {
		return fmt.Errorf("load worker connection: %w", err)
	}
	storedKey, foundKey, err := localStore.LoadSigningKey()
	if err != nil {
		return fmt.Errorf("load worker signing key: %w", err)
	}
	if bootstrap != nil && needsRunnerEnrollment(bootstrap, found, foundKey, storedKey) {
		bootstrap.ControlPlaneURL = resolveControlPlaneEndpoint(bootstrap)
		enrollmentKey := storedKey
		enrollmentKey, err = protocol.GenerateSigningKey("runner:"+bootstrap.RunnerID, time.Now().UTC(), 365*24*time.Hour)
		if err != nil {
			return fmt.Errorf("generate enrollment signing key: %w", err)
		}
		bootstrap.RunnerKeyID = enrollmentKey.ID
		bootstrap.RunnerPublicKey = base64.RawStdEncoding.EncodeToString(enrollmentKey.Public.PublicKey)
		if enrolledConnection, enrollErr := bootstrap.Enroll(ctx); enrollErr == nil {
			connection = enrolledConnection
			if err := localStore.SaveSigningKey(enrollmentKey); err != nil {
				return fmt.Errorf("save enrolled signing key: %w", err)
			}
			if err := localStore.SaveConnection(connection); err != nil {
				return fmt.Errorf("save enrolled connection: %w", err)
			}
			storedKey, foundKey = enrollmentKey, true
		} else {
			return fmt.Errorf("runner enrollment failed: %w", enrollErr)
		}
	} else if !found {
		connection = worker.RunnerConnection{}
	}
	connection.NATSURL = resolveNATSEndpoint(bootstrap, connection.NATSURL)
	if bootstrap != nil && bootstrap.ControlPublicKey != "" && connection.ControlPublicKey != bootstrap.ControlPublicKey {
		connection.ControlPublicKey = bootstrap.ControlPublicKey
		if err := localStore.SaveConnection(connection); err != nil {
			return fmt.Errorf("save control-plane key: %w", err)
		}
	}
	if connection.RunnerID != "" {
		if err := setWorkerEnv("RUNNER_ID", connection.RunnerID); err != nil {
			return err
		}
	}
	if connection.NATSURL != "" {
		if err := setWorkerEnv("NATS_URL", connection.NATSURL); err != nil {
			return err
		}
	}
	if connection.MaxMessageBytes > 0 {
		if err := setWorkerEnv("MAX_MESSAGE_BYTES", fmt.Sprintf("%d", connection.MaxMessageBytes)); err != nil {
			return err
		}
		if os.Getenv("MAX_OUTPUT_BYTES") == "" {
			if err := setWorkerEnv("MAX_OUTPUT_BYTES", fmt.Sprintf("%d", connection.MaxMessageBytes)); err != nil {
				return err
			}
		}
	}
	controlPublicKey, err := base64.RawStdEncoding.DecodeString(connection.ControlPublicKey)
	if err != nil || len(controlPublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("runner control-plane public key is unavailable")
	}
	cfg, err := config.FromEnv(config.Worker)
	if err != nil {
		return fmt.Errorf("load worker configuration: %w", err)
	}
	if status != nil {
		status.SetRunnerID(cfg.RunnerID)
		endpoint, parseErr := redactNATSEndpoint(cfg.NATSURL)
		if parseErr != nil {
			fmt.Fprintln(stderr, "worker NATS endpoint could not be parsed")
		}
		status.SetNATSEndpoint(endpoint)
	}
	activeOrders := &worker.ActiveOrders{}
	workerKey := storedKey
	if !foundKey || workerKey.ID != "runner:"+cfg.RunnerID || time.Now().UTC().After(workerKey.Public.NotAfter) {
		workerKey, err = protocol.GenerateSigningKey("runner:"+cfg.RunnerID, time.Now().UTC(), 365*24*time.Hour)
		if err != nil {
			return fmt.Errorf("generate worker signing key: %w", err)
		}
		if err := localStore.SaveSigningKey(workerKey); err != nil {
			return fmt.Errorf("save worker signing key: %w", err)
		}
	}
	previousBootID, err := loadPreviousBootID(localStore)
	if err != nil {
		return err
	}
	if previousBootID != "" {
		if _, err := worker.RecoverDurableSigned(localStore, previousBootID, workerKey); err != nil {
			return fmt.Errorf("recover durable worker events: %w", err)
		}
	}
	bootID := fmt.Sprintf("%s-%d", cfg.RunnerID, time.Now().UnixNano())
	if err := localStore.Put("worker.boot", bootID); err != nil {
		return fmt.Errorf("save worker boot id: %w", err)
	}
	var jetstream *queue.JetStream
	closeJetStream := func() {
		if jetstream != nil {
			jetstream.Close()
			jetstream = nil
		}
	}
	defer closeJetStream()
	if strings.HasPrefix(cfg.NATSURL, "tls://") {
		jetstream, err = queue.ConnectJetStreamTLS(cfg.NATSURL, queue.TLSConfig{CertificateFile: cfg.NATSCertFile, KeyFile: cfg.NATSKeyFile, CAFile: cfg.NATSCAFile})
	} else {
		jetstream, err = queue.ConnectJetStreamPlain(cfg.NATSURL)
	}
	if err != nil {
		return fmt.Errorf("connect to NATS JetStream: %w", err)
	}
	capacity := connection.Capacity
	if capacity < 1 {
		capacity = store.DefaultRunnerCapacity
	}
	var currentCapacity atomic.Int64
	currentCapacity.Store(int64(capacity))
	if status != nil {
		status.SetCapacitySource(&currentCapacity)
		status.SetRunningSource(activeOrders.Count)
	}
	runtime := worker.OrderRuntime{Store: localStore, Publisher: jetstream, StartClaimer: worker.NewNATSStartClaimer(jetstream, workerKey, ed25519.PublicKey(controlPublicKey)), RunnerID: cfg.RunnerID, ExecutorBootID: bootID, ProcessID: int64(os.Getpid()), ControlPublicKey: ed25519.PublicKey(controlPublicKey), SigningKey: workerKey, Active: activeOrders, Executor: worker.Executor{Roots: []string{cfg.DataDir, "."}, MaxOutputBytes: cfg.MaxOutputBytes}, Writer: stdout}
	consumer, err := jetstream.Consumer(ctx, "runner-"+cfg.RunnerID, queue.Subject("orders", cfg.RunnerID), queue.UnlimitedPending)
	if err != nil {
		return fmt.Errorf("create order consumer: %w", err)
	}
	controlConsumer, err := jetstream.Consumer(ctx, "runner-control-"+cfg.RunnerID, queue.Subject("control", cfg.RunnerID), 10)
	if err != nil {
		return fmt.Errorf("create control consumer: %w", err)
	}
	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		for ctx.Err() == nil {
			if err := jetstream.ConsumeConcurrent(ctx, consumer, func(handlerCtx context.Context, message queue.Message) error {
				return runtime.Handle(handlerCtx, message)
			}); err != nil && ctx.Err() == nil {
				fmt.Fprintln(stderr, "runner order consumer:", err)
				time.Sleep(time.Second)
			}
		}
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		for ctx.Err() == nil {
			if err := worker.PublishPendingEvents(ctx, localStore, jetstream, cfg.RunnerID); err != nil && ctx.Err() == nil {
				fmt.Fprintln(stderr, "runner event publisher:", err)
				time.Sleep(time.Second)
			}
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		workerHeartbeat(ctx, jetstream, cfg.RunnerID, bootID, workerKey, &currentCapacity, stderr)
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		for ctx.Err() == nil {
			if err := jetstream.ConsumeOne(ctx, controlConsumer, func(handlerCtx context.Context, message queue.Message) error {
				return worker.ApplyRunnerControl(handlerCtx, message, cfg.RunnerID, ed25519.PublicKey(controlPublicKey), &currentCapacity)
			}); err != nil && ctx.Err() == nil {
				fmt.Fprintln(stderr, "runner control consumer:", err)
				time.Sleep(time.Second)
			}
		}
	}()
	fmt.Fprintf(stdout, "Glyphflow worker v%s\n", backend.Version)
	<-ctx.Done()
	closeJetStream()
	background.Wait()
	return nil
}

var setenv = os.Setenv

func setWorkerEnv(name, value string) error {
	if err := setenv(name, value); err != nil {
		return fmt.Errorf("set worker environment %s: %w", name, err)
	}
	return nil
}

func loadPreviousBootID(localStore *worker.LocalStore) (string, error) {
	raw, err := localStore.Get("worker.boot")
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read worker boot metadata: %w", err)
	}
	var bootID string
	if err := json.Unmarshal(raw, &bootID); err != nil {
		return "", fmt.Errorf("decode worker boot metadata: %w", err)
	}
	if strings.TrimSpace(bootID) == "" {
		return "", errors.New("worker boot metadata is empty")
	}
	return bootID, nil
}

func resolveNATSEndpoint(bootstrap *worker.Bootstrap, enrolled string) string {
	embedded := ""
	if bootstrap != nil {
		embedded = bootstrap.NATSURL
	}
	for _, endpoint := range []string{os.Getenv("GLYPHFLOW_NATS_ENDPOINT"), os.Getenv("RUNNER_NATS_URL"), embedded, enrolled} {
		if endpoint = strings.TrimSpace(endpoint); endpoint != "" {
			return endpoint
		}
	}
	return ""
}

func resolveControlPlaneEndpoint(bootstrap *worker.Bootstrap) string {
	embedded := ""
	if bootstrap != nil {
		embedded = bootstrap.ControlPlaneURL
	}
	for _, endpoint := range []string{os.Getenv("GLYPHFLOW_CONTROL_PLANE_URL"), os.Getenv("RUNNER_CONTROL_PLANE_URL"), embedded} {
		if endpoint = strings.TrimSpace(endpoint); endpoint != "" {
			return endpoint
		}
	}
	return ""
}

func workerHeartbeat(ctx context.Context, jetstream *queue.JetStream, runnerID, bootID string, signingKey protocol.SigningKey, capacity *atomic.Int64, stderr io.Writer) {
	if stderr == nil {
		stderr = io.Discard
	}
	publish := func(now time.Time) {
		payload, _ := json.Marshal(map[string]any{"runner_id": runnerID, "boot_id": bootID, "at": now.UTC().Format(time.RFC3339Nano), "capacity": capacity.Load()})
		envelope, err := signingKey.SignEvent(payload)
		if err != nil {
			return
		}
		raw, err := protocol.EncodeEnvelope(envelope)
		if err != nil {
			return
		}
		if err := jetstream.Publish(ctx, queue.Message{Subject: queue.Subject("events", runnerID), Data: raw, ID: "heartbeat:" + bootID + ":" + now.UTC().Format(time.RFC3339Nano)}); err != nil {
			fmt.Fprintln(stderr, "runner heartbeat:", err)
		}
	}
	publish(time.Now().UTC())
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			publish(now)
		}
	}
}

func needsRunnerEnrollment(bootstrap *worker.Bootstrap, connectionFound, keyFound bool, key protocol.SigningKey) bool {
	return bootstrap != nil && (!connectionFound || !keyFound || key.ID != "runner:"+bootstrap.RunnerID || time.Now().UTC().After(key.Public.NotAfter))
}
