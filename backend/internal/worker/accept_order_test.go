package worker

import (
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
)

func TestAcceptOrderVerifiesBeforePersistence(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	payload := protocol.OrderPayload{
		Version: protocol.ProtocolVersion, OrderID: "order-1", RunID: "run-1", TaskID: "task-1",
		Attempt: 1, LeaseToken: "lease-1", RunnerID: "runner-1", IssuedAt: now,
		NotBefore: now, ExpiresAt: now.Add(time.Minute), Type: protocol.OrderExecute,
		Command: []string{"echo", "ok"}, WorkingDir: t.TempDir(), TimeoutSeconds: 1,
		Limits: protocol.ResourceLimits{MaxOutputBytes: 1024},
	}
	payloadBytes, err := protocol.EncodeOrderPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope := protocol.NewEnvelope("control-plane", payloadBytes)
	if err := envelope.SignOrder(privateKey); err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(t.TempDir() + "/worker.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	keys := protocol.Keyring{"control-plane": {ID: "control-plane", PublicKey: publicKey}}
	if _, err := store.AcceptOrder(raw, keys, now, "runner-1", "run-1", 1, "lease-1", time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("order-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcceptOrder(raw, keys, now.Add(2*time.Minute), "runner-1", "run-1", 1, "lease-1", time.Second); err == nil {
		t.Fatal("expired order was accepted")
	}
	if _, err := store.Get("order-1"); err != nil {
		t.Fatal("valid order was removed after rejected replay")
	}
}
