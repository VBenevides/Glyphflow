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
	natsURL := os.Getenv("NATS_TLS_URL")
	if databaseURL == "" || natsURL == "" || os.Getenv("NATS_CERT_FILE") == "" || os.Getenv("NATS_KEY_FILE") == "" || os.Getenv("NATS_CA_FILE") == "" {
		t.Skip("set database and mutual-TLS NATS variables to run integration tests")
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
	jetstream, err := queue.ConnectJetStreamTLS(natsURL, queue.TLSConfig{CertificateFile: os.Getenv("NATS_CERT_FILE"), KeyFile: os.Getenv("NATS_KEY_FILE"), CAFile: os.Getenv("NATS_CA_FILE")})
	if err != nil {
		t.Fatal(err)
	}
	defer jetstream.Close()
	if err := jetstream.Publish(ctx, queue.Message{Subject: queue.Subject("orders", "integration"), ID: "integration-1", Data: []byte("signed")}); err != nil {
		t.Fatal(err)
	}
}
