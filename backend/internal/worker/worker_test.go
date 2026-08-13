package worker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecutorRejectsUntrustedDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := (Executor{Roots: []string{"/srv/tasks"}, MaxOutputBytes: 1024}).Run(ctx, []string{"echo", "ok"}, "/tmp"); err == nil {
		t.Fatal("untrusted directory accepted")
	}
}

func TestExecutorEnforcesCommandAndOutputLimits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	executor := Executor{Roots: []string{"/tmp"}, AllowedCommands: map[string]bool{"printf": true}, MaxOutputBytes: 4}
	if _, err := executor.Run(ctx, []string{"sh", "-c", "printf 12345"}, "/tmp"); err == nil {
		t.Fatal("disallowed executable accepted")
	}
	output, err := executor.Run(ctx, []string{"printf", "12345"}, "/tmp")
	if err != ErrOutputLimit || !strings.HasPrefix(string(output), "1234") {
		t.Fatalf("output limit failed: %q %v", output, err)
	}
}
