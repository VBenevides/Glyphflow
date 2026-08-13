package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOrderPayloadContainsExecutionData(t *testing.T) {
	want := OrderPayload{
		Version:        ProtocolVersion,
		OrderID:        "order-1",
		RunID:          "run-1",
		TaskID:         "task-1",
		Attempt:        1,
		LeaseToken:     "lease-1",
		RunnerID:       "worker-1",
		IssuedAt:       time.Unix(100, 0).UTC(),
		NotBefore:      time.Unix(101, 0).UTC(),
		ExpiresAt:      time.Unix(200, 0).UTC(),
		Command:        []string{"echo", "hello"},
		WorkingDir:     "/srv/tasks",
		SecretRefs:     []string{"db-password"},
		TimeoutSeconds: 30,
		Limits:         ResourceLimits{MaxOutputBytes: 1024, MaxMemoryBytes: 2048, MaxProcesses: 2},
		Resources:      map[string]string{"pool": "default"},
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got OrderPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.OrderID != want.OrderID || got.RunID != want.RunID || got.RunnerID != want.RunnerID || got.TimeoutSeconds != want.TimeoutSeconds {
		t.Fatalf("payload fields were not preserved: %#v", got)
	}
}
