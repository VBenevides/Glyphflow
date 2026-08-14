package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/config"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	bootstrap, err := worker.LoadEmbeddedBootstrap()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if os.Getenv("DATA_DIR") == "" {
		dataDir := worker.DefaultDataDir()
		if bootstrap != nil {
			dataDir = filepath.Join(dataDir, bootstrap.RunnerID)
		}
		_ = os.Setenv("DATA_DIR", dataDir)
	}
	if err := os.MkdirAll(os.Getenv("DATA_DIR"), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	localStore, err := worker.OpenStore(filepath.Join(os.Getenv("DATA_DIR"), "runner.sqlite"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer localStore.Close()
	connection, found, err := localStore.LoadConnection()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !found {
		if bootstrap == nil {
			connection = worker.RunnerConnection{}
		} else if connection, err = bootstrap.Enroll(ctx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else if err := localStore.SaveConnection(connection); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if bootstrap != nil && bootstrap.ControlPublicKey != "" && connection.ControlPublicKey != bootstrap.ControlPublicKey {
		connection.ControlPublicKey = bootstrap.ControlPublicKey
		if err := localStore.SaveConnection(connection); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if connection.RunnerID != "" {
		_ = os.Setenv("RUNNER_ID", connection.RunnerID)
	}
	if connection.NATSURL != "" {
		_ = os.Setenv("NATS_URL", connection.NATSURL)
	}
	if connection.MaxMessageBytes > 0 {
		_ = os.Setenv("MAX_MESSAGE_BYTES", fmt.Sprintf("%d", connection.MaxMessageBytes))
		if os.Getenv("MAX_OUTPUT_BYTES") == "" {
			_ = os.Setenv("MAX_OUTPUT_BYTES", fmt.Sprintf("%d", connection.MaxMessageBytes))
		}
	}
	controlPublicKey, err := base64.RawStdEncoding.DecodeString(connection.ControlPublicKey)
	if err != nil || len(controlPublicKey) != ed25519.PublicKeySize {
		fmt.Fprintln(os.Stderr, "runner control-plane public key is unavailable")
		os.Exit(1)
	}
	cfg, err := config.FromEnv(config.Worker)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var previousBootID string
	if raw, err := localStore.Get("worker.boot"); err == nil {
		_ = json.Unmarshal(raw, &previousBootID)
	}
	if previousBootID != "" {
		if _, err := worker.RecoverDurable(localStore, previousBootID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	bootID := fmt.Sprintf("%s-%d", cfg.RunnerID, time.Now().UnixNano())
	if err := localStore.Put("worker.boot", bootID); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	workerKey, err := protocol.GenerateSigningKey("runner:"+cfg.RunnerID, time.Now().UTC(), 365*24*time.Hour)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var jetstream *queue.JetStream
	if strings.HasPrefix(cfg.NATSURL, "tls://") {
		jetstream, err = queue.ConnectJetStreamTLS(cfg.NATSURL, queue.TLSConfig{CertificateFile: cfg.NATSCertFile, KeyFile: cfg.NATSKeyFile, CAFile: cfg.NATSCAFile})
	} else {
		jetstream, err = queue.ConnectJetStreamPlain(cfg.NATSURL)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer jetstream.Close()
	runtime := worker.OrderRuntime{Store: localStore, Publisher: jetstream, RunnerID: cfg.RunnerID, ExecutorBootID: bootID, ProcessID: int64(os.Getpid()), ControlPublicKey: ed25519.PublicKey(controlPublicKey), SigningKey: workerKey, Executor: worker.Executor{Roots: []string{cfg.DataDir, "."}, MaxOutputBytes: cfg.MaxOutputBytes}}
	consumer, err := jetstream.Consumer(ctx, "runner-"+cfg.RunnerID, queue.Subject("orders", cfg.RunnerID), 100)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	go func() {
		for ctx.Err() == nil {
			if err := jetstream.ConsumeOne(ctx, consumer, func(handlerCtx context.Context, message queue.Message) error {
				return runtime.Handle(handlerCtx, message)
			}); err != nil && ctx.Err() == nil {
				time.Sleep(time.Second)
			}
		}
	}()
	go func() {
		for ctx.Err() == nil {
			if err := worker.PublishPendingEvents(ctx, localStore, jetstream, cfg.RunnerID); err != nil && ctx.Err() == nil {
				time.Sleep(time.Second)
			}
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()
	go workerHeartbeat(ctx, jetstream, cfg.RunnerID, bootID, workerKey)
	fmt.Println("Glyphflow worker")
	<-ctx.Done()
}

func workerHeartbeat(ctx context.Context, jetstream *queue.JetStream, runnerID, bootID string, signingKey protocol.SigningKey) {
	publicKey := signingKey.Private.Public().(ed25519.PublicKey)
	publish := func(now time.Time) {
		payload, _ := json.Marshal(map[string]string{"runner_id": runnerID, "boot_id": bootID, "at": now.UTC().Format(time.RFC3339Nano), "key_id": signingKey.ID, "public_key": base64.RawStdEncoding.EncodeToString(publicKey)})
		_ = jetstream.Publish(ctx, queue.Message{Subject: queue.Subject("events", runnerID), Data: payload, ID: "heartbeat:" + bootID + ":" + now.UTC().Format(time.RFC3339Nano)})
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
