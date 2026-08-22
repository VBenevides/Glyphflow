package controlplane

import (
	"errors"
	"strings"
	"testing"
)

func TestHealthRequiresEveryComponent(t *testing.T) {
	health := NewHealth("scheduler", "dispatcher")
	if err := health.Ready(); err == nil || !strings.Contains(err.Error(), "scheduler") {
		t.Fatalf("Ready() = %v, want missing scheduler", err)
	}
	health.MarkHealthy("scheduler")
	if err := health.Ready(); err == nil || !strings.Contains(err.Error(), "dispatcher") {
		t.Fatalf("Ready() = %v, want missing dispatcher", err)
	}
	health.MarkHealthy("dispatcher")
	if err := health.Ready(); err != nil {
		t.Fatal(err)
	}
}

func TestHealthReportsFailureUntilRecovery(t *testing.T) {
	health := NewHealth("scheduler")
	health.MarkFailed("scheduler", errors.New("database unavailable"))
	if err := health.Ready(); err == nil || !strings.Contains(err.Error(), "unhealthy") {
		t.Fatalf("Ready() = %v, want unhealthy", err)
	}
	health.MarkHealthy("scheduler")
	if err := health.Ready(); err != nil {
		t.Fatal(err)
	}
}
