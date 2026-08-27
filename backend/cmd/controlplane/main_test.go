package main

import (
	"context"
	"errors"
	"testing"
)

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
