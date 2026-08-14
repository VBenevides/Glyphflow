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
	cfg, err := config.FromEnv(config.Worker)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	store, err := worker.OpenStore(filepath.Join(cfg.DataDir, "runner.sqlite"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer store.Close()
	var previousBootID string
	if raw, err := store.Get("worker.boot"); err == nil {
		_ = json.Unmarshal(raw, &previousBootID)
	}
	if previousBootID != "" {
		if _, err := worker.RecoverDurable(store, previousBootID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	bootID := fmt.Sprintf("%s-%d", cfg.RunnerID, time.Now().UnixNano())
	if err := store.Put("worker.boot", bootID); err != nil {
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
				return store.Put(key, string(message.Data))
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
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			payload, _ := json.Marshal(map[string]string{"runner_id": runnerID, "boot_id": bootID, "at": now.UTC().Format(time.RFC3339Nano)})
			_ = jetstream.Publish(ctx, queue.Message{Subject: queue.Subject("events", runnerID), Data: payload, ID: "heartbeat:" + bootID + ":" + now.UTC().Format(time.RFC3339Nano)})
		}
	}
}
