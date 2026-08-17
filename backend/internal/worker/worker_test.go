package worker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestActiveOrdersCancelAfterCompletionIsIdempotent(t *testing.T) {
	active := &ActiveOrders{}
	called := false
	item := active.put("attempt-1", func() { called = true })
	active.remove("attempt-1")
	if active.cancel("attempt-1") {
		t.Fatal("completed order was reported active")
	}
	if item.cancelled.Load() || called {
		t.Fatal("completed order was cancelled")
	}
}

func TestMemoryStatsSamplesCurrentProcess(t *testing.T) {
	stats := &MemoryStats{}
	stats.Sample(int32(os.Getpid()))
	if stats.MaxBytes == 0 || stats.AverageBytes == 0 {
		t.Fatalf("memory sample = max %d, average %d", stats.MaxBytes, stats.AverageBytes)
	}
}

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

func TestExecutorUsesValidatedWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pwd is not a Windows command")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := (Executor{Roots: []string{dir}, AllowedCommands: map[string]bool{"pwd": true}, MaxOutputBytes: 1024}).Run(ctx, []string{"pwd"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(output)); got != filepath.Clean(dir) || got == os.TempDir() {
		t.Fatalf("command ran in %q, want %q", got, filepath.Clean(dir))
	}
}

func TestExecutorStreamsOutputBeforeProcessExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not a Windows command")
	}
	var chunks []string
	executor := Executor{Roots: []string{"/tmp"}, AllowedCommands: map[string]bool{"sh": true}, MaxOutputBytes: 1024}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := executor.RunStreaming(ctx, []string{"sh", "-c", "printf first; printf err >&2; sleep 0.03; printf second"}, "/tmp", 10*time.Millisecond, func(stream string, chunk []byte) error {
		chunks = append(chunks, stream+":"+string(chunk))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(chunks, "")
	if len(chunks) < 2 || !strings.Contains(joined, "stdout:first") || !strings.Contains(joined, "stderr:err") || !strings.Contains(joined, "stdout:second") {
		t.Fatalf("streamed chunks = %v", chunks)
	}
}

func TestExecutorReturnsNonZeroExitCodeAsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not a Windows command")
	}
	executor := Executor{Roots: []string{"/tmp"}, AllowedCommands: map[string]bool{"sh": true}, MaxOutputBytes: 1024}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, exitCode, err := executor.RunStreamingWithExitCode(ctx, []string{"sh", "-c", "exit 7"}, "/tmp", 0, nil)
	if err == nil || exitCode == nil || *exitCode != 7 {
		t.Fatalf("exit code = %v, err = %v", exitCode, err)
	}
}

func TestExecutorEnvironmentOverridesProcessValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not a Windows command")
	}
	executor := Executor{Roots: []string{"/tmp"}, AllowedCommands: map[string]bool{"sh": true}, MaxOutputBytes: 1024, Environment: map[string]string{"GLYPHFLOW_TEST": "task"}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := executor.Run(ctx, []string{"sh", "-c", "printf %s \"$GLYPHFLOW_TEST\""}, "/tmp")
	if err != nil || string(output) != "task" {
		t.Fatalf("environment = %q, err = %v", output, err)
	}
}
