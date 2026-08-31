package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func setControlPlaneStartupEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://127.0.0.1:1/glyphflow")
	t.Setenv("NATS_URL", "nats://127.0.0.1:1")
	t.Setenv("ACCESS_TOKEN_SECRET", "01234567890123456789012345678901")
	t.Setenv("PASSWORD_PEPPER", "0123456789012345")
	t.Setenv("WEB_ORIGIN", "http://localhost:3000")
	t.Setenv("ENVIRONMENT", "development")
	t.Setenv("ALLOW_INSECURE_TRANSPORT", "true")
	t.Setenv("DATA_DIR", t.TempDir())
	t.Setenv("MAX_MESSAGE_BYTES", "1048576")
}

func TestRunReturnsOnStartupCancellationAndClosesDatabase(t *testing.T) {
	setControlPlaneStartupEnv(t)
	previousNotifyContext := notifyContext
	notifyContext = func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx, func() {}
	}
	t.Cleanup(func() { notifyContext = previousNotifyContext })

	closed := false
	previousClose := closeControlPlaneDB
	closeControlPlaneDB = func(db *pgxpool.Pool) {
		previousClose(db)
		closed = true
	}
	t.Cleanup(func() { closeControlPlaneDB = previousClose })

	if err := run(); !errors.Is(err, context.Canceled) {
		t.Fatalf("run cancellation = %v", err)
	}
	if !closed {
		t.Fatal("run did not close the database pool")
	}
}

func TestRunReturnsOnDatabaseFailureAndClosesDatabase(t *testing.T) {
	setControlPlaneStartupEnv(t)
	previousNotifyContext := notifyContext
	notifyContext = func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 20*time.Millisecond)
	}
	t.Cleanup(func() { notifyContext = previousNotifyContext })

	closed := false
	previousClose := closeControlPlaneDB
	closeControlPlaneDB = func(db *pgxpool.Pool) {
		previousClose(db)
		closed = true
	}
	t.Cleanup(func() { closeControlPlaneDB = previousClose })

	if err := run(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("run database failure = %v", err)
	}
	if !closed {
		t.Fatal("run did not close the database pool")
	}
}

func TestWaitForDatabaseRetries(t *testing.T) {
	attempts := 0
	err := waitForDatabase(context.Background(), func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("database unavailable")
		}
		return nil
	}, 0)
	if err != nil || attempts != 3 {
		t.Fatalf("waitForDatabase = %v after %d attempts", err, attempts)
	}
}

func TestWaitForDatabaseStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForDatabase(ctx, func(context.Context) error {
		return errors.New("database unavailable")
	}, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForDatabase cancellation = %v", err)
	}
}
