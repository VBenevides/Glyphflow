package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOrderPayloadContainsExecutionData(t *testing.T) {
	want := OrderPayload{
		Version:         ProtocolVersion,
		OrderID:         "order-1",
		RunID:           "run-1",
		TaskID:          "task-1",
		TaskName:        "Example task",
		TaskVersion:     2,
		Attempt:         1,
		LeaseToken:      "lease-1",
		RunnerID:        "worker-1",
		IssuedAt:        time.Unix(100, 0).UTC(),
		NotBefore:       time.Unix(101, 0).UTC(),
		ExpiresAt:       time.Unix(200, 0).UTC(),
		Command:         []string{"echo", "hello"},
		WorkingDir:      "/srv/tasks",
		SecretRefs:      map[string]string{"DB_PASSWORD": "db-password"},
		DurationSeconds: 30,
		Limits:          ResourceLimits{MaxOutputBytes: 1024, MaxMemoryBytes: 2048, MaxProcesses: 2},
		Resources:       map[string]string{"pool": "default"},
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got OrderPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.OrderID != want.OrderID || got.RunID != want.RunID || got.RunnerID != want.RunnerID || got.TaskName != want.TaskName || got.TaskVersion != want.TaskVersion || got.DurationSeconds != want.DurationSeconds {
		t.Fatalf("payload fields were not preserved: %#v", got)
	}
}

func TestEventPayloadContainsLifecycleData(t *testing.T) {
	want := EventPayload{
		Version:      ProtocolVersion,
		EventID:      "event-1",
		OrderID:      "order-1",
		RunID:        "run-1",
		TaskID:       "task-1",
		Attempt:      1,
		LeaseToken:   "lease-1",
		RunnerID:     "worker-1",
		Sequence:     3,
		ObservedAt:   time.Unix(200, 0).UTC(),
		Result:       "success",
		Metrics:      map[string]int64{"duration_ms": 12},
		OutputDigest: "sha256:abc",
		Error:        "",
	}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got EventPayload
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.EventID != want.EventID || got.RunID != want.RunID || got.Sequence != want.Sequence || got.OutputDigest != want.OutputDigest {
		t.Fatalf("event fields were not preserved: %#v", got)
	}
}

func TestRunnerControlPayloadContainsCapacity(t *testing.T) {
	want := RunnerControlPayload{Version: ProtocolVersion, Type: RunnerControlCapacity, RunnerID: "runner-1", Capacity: 42, IssuedAt: time.Unix(300, 0).UTC()}
	raw, err := EncodeRunnerControlPayload(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeRunnerControlPayload(raw)
	if err != nil || got != want {
		t.Fatalf("runner control payload = %#v, err=%v", got, err)
	}
}
