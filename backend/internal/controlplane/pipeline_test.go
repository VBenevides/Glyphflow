package controlplane

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/queue"
	"github.com/VBenevides/Glyphflow/backend/internal/worker"
)

func TestPipelineExecutesOneVerifiedTask(t *testing.T) {
	controlPublic, controlPrivate, _ := ed25519.GenerateKey(nil)
	workerPublic, workerPrivate, _ := ed25519.GenerateKey(nil)
	_ = controlPublic
	_ = workerPublic
	store, err := worker.OpenStore(t.TempDir() + "/worker.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	dir := t.TempDir()
	pipeline := Pipeline{ControlPrivate: controlPrivate, WorkerPrivate: workerPrivate, Queue: queue.NewMemory(), Store: store, Executor: worker.Executor{Roots: []string{dir}, AllowedCommands: map[string]bool{"printf": true}, MaxOutputBytes: 1024}}
	result, err := pipeline.Execute(context.Background(), TaskRequest{OrderID: "order-1", RunID: "run-1", TaskID: "task-1", RunnerID: "worker-1", LeaseToken: "lease-1", Command: []string{"printf", "ok"}, WorkingDir: dir, Timeout: time.Second, MaxOutput: 1024})
	if err != nil || result.State != "completed" || string(result.Output) != "ok" {
		t.Fatalf("pipeline failed: %#v %v", result, err)
	}
}
