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
	"net/url"
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
	"github.com/nats-io/nats.go/jetstream"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

const runnerKeyPrefix = "runner:"

func runWorker(ctx context.Context, stdout, stderr io.Writer, status StatusSink) error {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	bootstrap, err := loadEmbeddedBootstrap()
	if err != nil {
		return fmt.Errorf("load worker bootstrap: %w", err)
	}
	if err := applyBootstrapTransportDefaults(bootstrap); err != nil {
		return err
	}
	if err := prepareWorkerDataDir(bootstrap); err != nil {
		return err
	}
	localStore, err := worker.OpenStore(filepath.Join(os.Getenv("DATA_DIR"), "runner.sqlite"))
	if err != nil {
		return fmt.Errorf("open worker store: %w", err)
	}
	defer closeWorkerStore(localStore)
	connection, found, storedKey, foundKey, err := loadWorkerState(localStore)
	if err != nil {
		return err
	}
	connection, storedKey, foundKey, err = enrollWorkerIfNeeded(ctx, bootstrap, localStore, connection, found, storedKey, foundKey)
	if err != nil {
		return err
	}
	connection, err = configureWorkerConnection(bootstrap, localStore, connection)
	if err != nil {
		return err
	}
	controlPublicKey, err := base64.RawStdEncoding.DecodeString(connection.ControlPublicKey)
	if err != nil || len(controlPublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("runner control-plane public key is unavailable")
	}
	cfg, err := config.FromEnv(config.Worker)
	if err != nil {
		return fmt.Errorf("load worker configuration: %w", err)
	}
	setWorkerStatus(status, cfg, stderr)
	return runWorkerProcess(ctx, stdout, stderr, status, localStore, cfg, connection, storedKey, foundKey, controlPublicKey)
}

func prepareWorkerDataDir(bootstrap *worker.Bootstrap) error {
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
	return nil
}

func loadWorkerState(localStore *worker.LocalStore) (worker.RunnerConnection, bool, protocol.SigningKey, bool, error) {
	connection, found, err := localStore.LoadConnection()
	if err != nil {
		return worker.RunnerConnection{}, false, protocol.SigningKey{}, false, fmt.Errorf("load worker connection: %w", err)
	}
	storedKey, foundKey, err := localStore.LoadSigningKey()
	if err != nil {
		return worker.RunnerConnection{}, false, protocol.SigningKey{}, false, fmt.Errorf("load worker signing key: %w", err)
	}
	return connection, found, storedKey, foundKey, nil
}

func enrollWorkerIfNeeded(ctx context.Context, bootstrap *worker.Bootstrap, localStore *worker.LocalStore, connection worker.RunnerConnection, found bool, storedKey protocol.SigningKey, foundKey bool) (worker.RunnerConnection, protocol.SigningKey, bool, error) {
	if bootstrap != nil && needsRunnerEnrollment(bootstrap, found, foundKey, storedKey) {
		bootstrap.ControlPlaneURL = resolveControlPlaneEndpoint(bootstrap)
		if err := validateWorkerControlPlaneEndpoint(bootstrap.ControlPlaneURL); err != nil {
			return connection, storedKey, foundKey, fmt.Errorf("worker control-plane endpoint: %w", err)
		}
		enrollmentKey, err := protocol.GenerateSigningKey(runnerKeyPrefix+bootstrap.RunnerID, time.Now().UTC(), 365*24*time.Hour)
		if err != nil {
			return connection, storedKey, foundKey, fmt.Errorf("generate enrollment signing key: %w", err)
		}
		bootstrap.RunnerKeyID = enrollmentKey.ID
		bootstrap.RunnerPublicKey = base64.RawStdEncoding.EncodeToString(enrollmentKey.Public.PublicKey)
		enrolledConnection, enrollErr := bootstrap.Enroll(ctx)
		if enrollErr == nil {
			connection = enrolledConnection
			if err := localStore.SaveSigningKey(enrollmentKey); err != nil {
				return connection, storedKey, foundKey, fmt.Errorf("save enrolled signing key: %w", err)
			}
			if err := localStore.SaveConnection(connection); err != nil {
				return connection, storedKey, foundKey, fmt.Errorf("save enrolled connection: %w", err)
			}
			return connection, enrollmentKey, true, nil
		}
		return connection, storedKey, foundKey, fmt.Errorf("runner enrollment failed: %w", enrollErr)
	}
	if !found {
		connection = worker.RunnerConnection{}
	}
	return connection, storedKey, foundKey, nil
}

func configureWorkerConnection(bootstrap *worker.Bootstrap, localStore *worker.LocalStore, connection worker.RunnerConnection) (worker.RunnerConnection, error) {
	connection.NATSURL = resolveNATSEndpoint(bootstrap, connection.NATSURL)
	if bootstrap != nil && bootstrap.ControlPublicKey != "" && connection.ControlPublicKey != bootstrap.ControlPublicKey {
		connection.ControlPublicKey = bootstrap.ControlPublicKey
		if err := localStore.SaveConnection(connection); err != nil {
			return connection, fmt.Errorf("save control-plane key: %w", err)
		}
	}
	if connection.RunnerID != "" {
		if err := setWorkerEnv("RUNNER_ID", connection.RunnerID); err != nil {
			return connection, err
		}
	}
	if connection.NATSURL != "" {
		if err := setWorkerEnv("NATS_URL", connection.NATSURL); err != nil {
			return connection, err
		}
	}
	if connection.MaxMessageBytes > 0 {
		if err := setWorkerEnv("MAX_MESSAGE_BYTES", fmt.Sprintf("%d", connection.MaxMessageBytes)); err != nil {
			return connection, err
		}
		if os.Getenv("MAX_OUTPUT_BYTES") == "" {
			if err := setWorkerEnv("MAX_OUTPUT_BYTES", fmt.Sprintf("%d", connection.MaxMessageBytes)); err != nil {
				return connection, err
			}
		}
	}
	return connection, nil
}

func setWorkerStatus(status StatusSink, cfg config.Config, stderr io.Writer) {
	if status == nil {
		return
	}
	status.SetRunnerID(cfg.RunnerID)
	endpoint, parseErr := redactNATSEndpoint(cfg.NATSURL)
	if parseErr != nil {
		fmt.Fprintln(stderr, "worker NATS endpoint could not be parsed")
	}
	status.SetNATSEndpoint(endpoint)
}

func runWorkerProcess(ctx context.Context, stdout, stderr io.Writer, status StatusSink, localStore *worker.LocalStore, cfg config.Config, connection worker.RunnerConnection, storedKey protocol.SigningKey, foundKey bool, controlPublicKey []byte) error {
	activeOrders := &worker.ActiveOrders{}
	workerKey, err := ensureWorkerSigningKey(localStore, cfg.RunnerID, storedKey, foundKey)
	if err != nil {
		return err
	}
	if err := recoverPreviousWorkerEvents(localStore, workerKey); err != nil {
		return err
	}
	bootID := fmt.Sprintf("%s-%d", cfg.RunnerID, time.Now().UnixNano())
	if err := localStore.Put("worker.boot", bootID); err != nil {
		return fmt.Errorf("save worker boot id: %w", err)
	}
	jetstream, err := connectWorkerJetStream(ctx, cfg)
	if err != nil {
		return err
	}
	jetstreamClosed := false
	closeJetStream := func() {
		if jetstream != nil && !jetstreamClosed {
			jetstream.Close()
			jetstreamClosed = true
		}
	}
	defer closeJetStream()
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
	runtime := worker.OrderRuntime{Store: localStore, Publisher: jetstream, StartClaimer: worker.NewNATSStartClaimer(jetstream, workerKey, ed25519.PublicKey(controlPublicKey)), SecretFetcher: worker.NewNATSSecretFetcher(jetstream, workerKey, ed25519.PublicKey(controlPublicKey)), RunnerID: cfg.RunnerID, ExecutorBootID: bootID, ProcessID: int64(os.Getpid()), ControlPublicKey: ed25519.PublicKey(controlPublicKey), SigningKey: workerKey, Active: activeOrders, Executor: worker.Executor{Roots: []string{cfg.DataDir, "."}, MaxOutputBytes: cfg.MaxOutputBytes}, Writer: stdout}
	orderSlots := make(chan struct{}, capacity)
	consumer, err := createJetStreamConsumer(jetstream, ctx, "runner-"+cfg.RunnerID, queue.Subject("orders", cfg.RunnerID), capacity)
	if err != nil {
		return fmt.Errorf("create order consumer: %w", err)
	}
	controlConsumer, err := createJetStreamConsumer(jetstream, ctx, "runner-control-"+cfg.RunnerID, queue.Subject("control", cfg.RunnerID), 10)
	if err != nil {
		return fmt.Errorf("create control consumer: %w", err)
	}
	var background sync.WaitGroup
	background.Add(1)
	go func() {
		defer background.Done()
		runOrderConsumer(ctx, jetstream, consumer, runtime, orderSlots, stderr)
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		runEventPublisher(ctx, localStore, jetstream, cfg.RunnerID, stderr)
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		workerHeartbeat(ctx, jetstream, cfg.RunnerID, bootID, workerKey, &currentCapacity, stderr)
	}()
	background.Add(1)
	go func() {
		defer background.Done()
		runControlConsumer(ctx, jetstream, controlConsumer, cfg.RunnerID, controlPublicKey, &currentCapacity, stderr)
	}()
	fmt.Fprintf(stdout, "Glyphflow worker v%s\n", backend.Version)
	<-ctx.Done()
	closeJetStream()
	background.Wait()
	return nil
}

func ensureWorkerSigningKey(localStore *worker.LocalStore, runnerID string, storedKey protocol.SigningKey, foundKey bool) (protocol.SigningKey, error) {
	workerKey := storedKey
	if !foundKey || workerKey.ID != runnerKeyPrefix+runnerID || time.Now().UTC().After(workerKey.Public.NotAfter) {
		var err error
		workerKey, err = protocol.GenerateSigningKey(runnerKeyPrefix+runnerID, time.Now().UTC(), 365*24*time.Hour)
		if err != nil {
			return protocol.SigningKey{}, fmt.Errorf("generate worker signing key: %w", err)
		}
		if err := localStore.SaveSigningKey(workerKey); err != nil {
			return protocol.SigningKey{}, fmt.Errorf("save worker signing key: %w", err)
		}
	}
	return workerKey, nil
}

func recoverPreviousWorkerEvents(localStore *worker.LocalStore, workerKey protocol.SigningKey) error {
	previousBootID, err := loadPreviousBootID(localStore)
	if err != nil {
		return err
	}
	if previousBootID == "" {
		return nil
	}
	if _, err := worker.RecoverDurableSigned(localStore, previousBootID, workerKey); err != nil {
		return fmt.Errorf("recover durable worker events: %w", err)
	}
	return nil
}

func connectWorkerJetStream(ctx context.Context, cfg config.Config) (*queue.JetStream, error) {
	var jetstream *queue.JetStream
	var err error
	if strings.HasPrefix(cfg.NATSURL, "tls://") {
		jetstream, err = connectJetStreamTLS(ctx, cfg.NATSURL, queue.TLSConfig{CertificateFile: cfg.NATSCertFile, KeyFile: cfg.NATSKeyFile, CAFile: cfg.NATSCAFile})
	} else {
		jetstream, err = connectJetStreamPlain(ctx, cfg.NATSURL)
	}
	if err != nil {
		if jetstream != nil {
			jetstream.Close()
		}
		return nil, fmt.Errorf("connect to NATS JetStream: %w", err)
	}
	return jetstream, nil
}

func runOrderConsumer(ctx context.Context, jetstream *queue.JetStream, consumer jetstream.Consumer, runtime worker.OrderRuntime, orderSlots chan struct{}, stderr io.Writer) {
	for ctx.Err() == nil {
		if err := jetstream.ConsumeConcurrent(ctx, consumer, func(handlerCtx context.Context, message queue.Message) error {
			select {
			case orderSlots <- struct{}{}:
			case <-handlerCtx.Done():
				return handlerCtx.Err()
			}
			defer func() { <-orderSlots }()
			return runtime.Handle(handlerCtx, message)
		}); err != nil && ctx.Err() == nil {
			fmt.Fprintln(stderr, "runner order consumer:", err)
			time.Sleep(time.Second)
		}
	}
}

func runEventPublisher(ctx context.Context, localStore *worker.LocalStore, jetstream *queue.JetStream, runnerID string, stderr io.Writer) {
	for ctx.Err() == nil {
		if err := worker.PublishPendingEvents(ctx, localStore, jetstream, runnerID); err != nil && ctx.Err() == nil {
			fmt.Fprintln(stderr, "runner event publisher:", err)
			time.Sleep(time.Second)
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return
		}
	}
}

func runControlConsumer(ctx context.Context, jetstream *queue.JetStream, consumer jetstream.Consumer, runnerID string, controlPublicKey []byte, currentCapacity *atomic.Int64, stderr io.Writer) {
	for ctx.Err() == nil {
		if err := jetstream.ConsumeOne(ctx, consumer, func(handlerCtx context.Context, message queue.Message) error {
			return worker.ApplyRunnerControl(handlerCtx, message, runnerID, ed25519.PublicKey(controlPublicKey), currentCapacity)
		}); err != nil && ctx.Err() == nil {
			fmt.Fprintln(stderr, "runner control consumer:", err)
			time.Sleep(time.Second)
		}
	}
}

var setenv = os.Setenv

var closeWorkerStore = func(localStore *worker.LocalStore) { _ = localStore.Close() }
var loadEmbeddedBootstrap = worker.LoadEmbeddedBootstrap
var connectJetStreamTLS = queue.ConnectJetStreamTLSWithContext
var connectJetStreamPlain = queue.ConnectJetStreamPlainWithContext
var createJetStreamConsumer = func(j *queue.JetStream, ctx context.Context, durable, subject string, maxPending int) (jetstream.Consumer, error) {
	return j.Consumer(ctx, durable, subject, maxPending)
}

func setWorkerEnv(name, value string) error {
	if err := setenv(name, value); err != nil {
		return fmt.Errorf("set worker environment %s: %w", name, err)
	}
	return nil
}

func applyBootstrapTransportDefaults(bootstrap *worker.Bootstrap) error {
	if bootstrap == nil || !bootstrap.AllowInsecureTransport {
		return nil
	}
	if os.Getenv("ENVIRONMENT") == "" {
		if err := setWorkerEnv("ENVIRONMENT", "development"); err != nil {
			return err
		}
	}
	if os.Getenv("ALLOW_INSECURE_TRANSPORT") == "" {
		if err := setWorkerEnv("ALLOW_INSECURE_TRANSPORT", "true"); err != nil {
			return err
		}
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

func validateWorkerControlPlaneEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return errors.New("must be a URL with a host and no credentials")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" && strings.EqualFold(strings.TrimSpace(os.Getenv("ENVIRONMENT")), "development") && strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_INSECURE_TRANSPORT")), "true") {
		return nil
	}
	return errors.New("must use HTTPS outside development")
}

type runnerResourceMetrics struct {
	CPUPercent       float64
	MemoryPercent    float64
	MemoryUsedBytes  int64
	MemoryTotalBytes int64
}

func sampleRunnerResources() *runnerResourceMetrics {
	memory, err := mem.VirtualMemory()
	if err != nil || memory.Total == 0 {
		return nil
	}
	cpuPercent := 0.0
	if values, err := cpu.Percent(100*time.Millisecond, false); err == nil && len(values) > 0 {
		cpuPercent = values[0]
	}
	return &runnerResourceMetrics{CPUPercent: cpuPercent, MemoryPercent: memory.UsedPercent, MemoryUsedBytes: int64(memory.Used), MemoryTotalBytes: int64(memory.Total)}
}

func runnerHeartbeatPayload(runnerID, bootID string, now time.Time, capacity int64, metrics *runnerResourceMetrics) []byte {
	payload := map[string]any{"runner_id": runnerID, "boot_id": bootID, "at": now.UTC().Format(time.RFC3339Nano), "capacity": capacity}
	if metrics != nil {
		payload["cpu_percent"] = metrics.CPUPercent
		payload["memory_percent"] = metrics.MemoryPercent
		payload["memory_used_bytes"] = metrics.MemoryUsedBytes
		payload["memory_total_bytes"] = metrics.MemoryTotalBytes
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func workerHeartbeat(ctx context.Context, jetstream *queue.JetStream, runnerID, bootID string, signingKey protocol.SigningKey, capacity *atomic.Int64, stderr io.Writer) {
	if stderr == nil {
		stderr = io.Discard
	}
	publish := func(now time.Time) {
		payload := runnerHeartbeatPayload(runnerID, bootID, now, capacity.Load(), sampleRunnerResources())
		envelope, err := signingKey.SignEvent(payload)
		if err != nil {
			return
		}
		raw, err := protocol.EncodeEnvelope(envelope)
		if err != nil {
			return
		}
		if err := jetstream.Publish(ctx, queue.Message{Subject: queue.Subject("heartbeats", runnerID), Data: raw, ID: "heartbeat:" + bootID + ":" + now.UTC().Format(time.RFC3339Nano)}); err != nil {
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
	return bootstrap != nil && (!connectionFound || !keyFound || key.ID != runnerKeyPrefix+bootstrap.RunnerID || time.Now().UTC().After(key.Public.NotAfter))
}
