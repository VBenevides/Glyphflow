package worker

import (
	"context"
	"testing"
)

func TestExecutorRejectsUntrustedDirectory(t *testing.T) {
	if _, err := (Executor{Roots: []string{"/srv/tasks"}}).Run(context.Background(), []string{"echo", "ok"}, "/tmp"); err == nil {
		t.Fatal("untrusted directory accepted")
	}
}
