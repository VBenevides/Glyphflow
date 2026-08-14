package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/config"
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
	consumer, err := jetstream.Consumer(ctx, "runner-"+cfg.RunnerID, queue.Subject("orders", cfg.RunnerID), 100)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	go func() {
		for ctx.Err() == nil {
			if err := jetstream.ConsumeOne(ctx, consumer, func(_ context.Context, message queue.Message) error {
				key := "order:" + message.ID
				if key == "order:" {
					key += fmt.Sprintf("%d", time.Now().UnixNano())
				}
				return localStore.Put(key, string(message.Data))
			}); err != nil && ctx.Err() == nil {
				time.Sleep(time.Second)
			}
		}
	}()
	go workerHeartbeat(ctx, jetstream, cfg.RunnerID, bootID)
	fmt.Println("Glyphflow worker")
	<-ctx.Done()
}

func workerHeartbeat(ctx context.Context, jetstream *queue.JetStream, runnerID, bootID string) {
	publish := func(now time.Time) {
		payload, _ := json.Marshal(map[string]string{"runner_id": runnerID, "boot_id": bootID, "at": now.UTC().Format(time.RFC3339Nano)})
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
