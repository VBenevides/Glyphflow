//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAndJetStreamDurableBoundary(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	natsURL := os.Getenv("NATS_URL")
	if databaseURL == "" || natsURL == "" {
		t.Skip("set DATABASE_URL and NATS_URL to run integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := store.ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	jetstream, err := queue.ConnectJetStream(natsURL)
	if err != nil {
		t.Fatal(err)
	}
	defer jetstream.Close()
	if err := jetstream.Publish(ctx, queue.Message{Subject: queue.Subject("orders", "integration"), ID: "integration-1", Data: []byte("signed")}); err != nil {
		t.Fatal(err)
	}
}
