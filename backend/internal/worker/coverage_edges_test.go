package worker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

func testBootstrap() Bootstrap {
	return Bootstrap{Token: "token", RunnerID: "runner", ControlPlaneURL: "https://control.example", ControlPublicKey: base64.RawStdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize)), MaxMessageBytes: 1024}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

type startClaimErrorRequester struct{ err error }

func (r startClaimErrorRequester) Request(context.Context, queue.Message, time.Duration) (queue.Message, error) {
	return queue.Message{}, r.err
}

func TestBootstrapValidationAndEnrollmentEdges(t *testing.T) {
	for _, input := range []Bootstrap{{}, {Token: "token"}, {Token: "token", RunnerID: "runner", ControlPlaneURL: "https://control.example", MaxMessageBytes: 1}} {
		if _, err := PackBootstrap(nil, input); err == nil {
			t.Fatalf("incomplete bootstrap accepted: %#v", input)
		}
	}
	input := testBootstrap()
	malformed := append([]byte("{"), make([]byte, 8)...)
	binary.BigEndian.PutUint64(malformed[1:], 1)
	malformed = append(malformed, bootstrapMagic...)
	if _, err := UnpackBootstrap(malformed); err == nil {
		t.Fatal("malformed bootstrap accepted")
	}
	invalidLength := append([]byte("x"), make([]byte, 8)...)
	binary.BigEndian.PutUint64(invalidLength[1:], 2)
	invalidLength = append(invalidLength, bootstrapMagic...)
	if _, err := UnpackBootstrap(invalidLength); err == nil {
		t.Fatal("oversized bootstrap payload accepted")
	}
	if _, err := LoadEmbeddedBootstrap(); err != nil {
		t.Fatal(err)
	}
	if DefaultDataDir() == "" {
		t.Fatal("default data directory is empty")
	}

	responses := []struct {
		status int
		body   string
		valid  bool
	}{
		{status: http.StatusOK, body: `{"runner_id":"runner","nats_url":"nats://localhost:4222","max_message_bytes":2048,"capacity":3}`, valid: true},
		{status: http.StatusBadRequest, body: `{"error":"enrollment rejected"}`},
		{status: http.StatusBadRequest, body: `{}`},
		{status: http.StatusOK, body: `{`},
		{status: http.StatusOK, body: `{"runner_id":"runner"}`},
	}
	originalTransport := http.DefaultTransport
	defer func() { http.DefaultTransport = originalTransport }()
	for _, test := range responses {
		http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: test.status, Status: http.StatusText(test.status), Body: io.NopCloser(strings.NewReader(test.body))}, nil
		})
		input.ControlPlaneURL = "https://control.example"
		connection, err := input.Enroll(context.Background())
		if test.valid {
			if err != nil || connection.RunnerID != "runner" || connection.Capacity != 3 {
				t.Fatalf("enrollment = %#v, %v", connection, err)
			}
		} else if err == nil {
			t.Fatalf("invalid enrollment accepted: %#v", test)
		}
	}
	input.ControlPlaneURL = "://invalid"
	if _, err := input.Enroll(context.Background()); err == nil {
		t.Fatal("invalid enrollment URL accepted")
	}
}

func TestExecutorWriterAndBufferEdges(t *testing.T) {
	chunks := make(chan executorOutput, 1)
	var stopped atomic.Bool
	writer := executorStreamWriter{stream: "stdout", chunks: chunks, stopped: &stopped}
	if n, err := writer.Write([]byte("ok")); n != 2 || err != nil {
		t.Fatalf("writer = %d, %v", n, err)
	}
	stopped.Store(true)
	if n, err := writer.Write([]byte("blocked")); n != 0 || !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("stopped writer = %d, %v", n, err)
	}
	buffer := boundedBuffer{limit: 2}
	if n, err := buffer.Write([]byte("abcd")); n != 2 || !errors.Is(err, io.ErrShortWrite) || !buffer.exceeded || !bytes.Equal(buffer.Bytes(), []byte("ab")) {
		t.Fatalf("partial buffer = %d, %v, %#v", n, err, buffer)
	}
	if n, err := buffer.Write([]byte("x")); n != 0 || !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("full buffer = %d, %v", n, err)
	}
	if _, err := (Executor{Roots: []string{"/tmp"}, MaxOutputBytes: 1024}).RunStreaming(context.Background(), []string{"printf", "callback"}, "/tmp", 0, func(string, []byte) error { return errors.New("callback failed") }); err == nil {
		t.Fatal("callback error was ignored")
	}
}

func TestExecutorValidationAndStartEdges(t *testing.T) {
	deadline, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, test := range []struct {
		name     string
		executor Executor
		ctx      context.Context
		args     []string
		dir      string
	}{
		{name: "command", executor: Executor{Roots: []string{"/tmp"}, MaxOutputBytes: 1}, ctx: deadline, dir: "/tmp"},
		{name: "deadline", executor: Executor{Roots: []string{"/tmp"}, MaxOutputBytes: 1}, ctx: context.Background(), args: []string{"printf"}, dir: "/tmp"},
		{name: "output", executor: Executor{Roots: []string{"/tmp"}}, ctx: deadline, args: []string{"printf"}, dir: "/tmp"},
		{name: "command policy", executor: Executor{Roots: []string{"/tmp"}, AllowedCommands: map[string]bool{"echo": true}, MaxOutputBytes: 1}, ctx: deadline, args: []string{"printf"}, dir: "/tmp"},
		{name: "root", executor: Executor{Roots: []string{"/srv/tasks"}, MaxOutputBytes: 1}, ctx: deadline, args: []string{"printf"}, dir: "/tmp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.executor.Run(test.ctx, test.args, test.dir); err == nil {
				t.Fatal("invalid execution accepted")
			}
		})
	}
	if _, err := (Executor{Roots: []string{"/tmp"}, AllowedCommands: map[string]bool{"missing-command": true}, MaxOutputBytes: 1}).Run(deadline, []string{"missing-command"}, "/tmp"); err == nil {
		t.Fatal("missing executable started")
	}
	var stats *MemoryStats
	stats.Sample(-1)
	(&MemoryStats{}).Sample(-1)
}

func TestApplyRunnerControlValidationEdges(t *testing.T) {
	var capacity atomic.Int64
	key, err := protocol.GenerateSigningKey("control", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := protocol.EncodeRunnerControlPayload(protocol.RunnerControlPayload{Version: protocol.ProtocolVersion, Type: protocol.RunnerControlCapacity, RunnerID: "runner", Capacity: 4, IssuedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := key.SignEvent(payload)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	valid := queue.Message{Subject: queue.Subject("control", "runner"), Data: raw}
	for _, message := range []queue.Message{{}, {Subject: "wrong", Data: raw}, {Subject: valid.Subject, Data: []byte("bad")}} {
		public := ed25519.PublicKey(key.Public.PublicKey)
		if err := ApplyRunnerControl(context.Background(), message, "runner", public, &capacity); err == nil {
			t.Fatalf("invalid runner control accepted: %#v", message)
		}
	}
	if err := ApplyRunnerControl(context.Background(), valid, "runner", key.Public.PublicKey, nil); err == nil {
		t.Fatal("runner control without capacity accepted")
	}
	if err := ApplyRunnerControl(context.Background(), valid, "runner", key.Public.PublicKey, &capacity); err != nil {
		t.Fatal(err)
	}
}

func TestNATSStartClaimerErrorEdges(t *testing.T) {
	now := time.Now().UTC()
	workerKey, err := protocol.GenerateSigningKey("runner", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	controlKey, err := protocol.GenerateSigningKey("control", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claim := protocol.StartClaimPayload{Version: protocol.ProtocolVersion, RequestID: "request", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", Attempt: 1, FencingToken: 1, ExecutionSpecDigest: "digest", IssuedAt: now}
	if err := (*NATSStartClaimer)(nil).ClaimStart(context.Background(), claim); !errors.Is(err, ErrStartUnavailable) {
		t.Fatalf("nil claimer error = %v", err)
	}
	if err := NewNATSStartClaimer(nil, workerKey, controlKey.Public.PublicKey).ClaimStart(context.Background(), claim); !errors.Is(err, ErrStartUnavailable) {
		t.Fatalf("nil requester error = %v", err)
	}
	if err := NewNATSStartClaimer(startClaimErrorRequester{err: errors.New("offline")}, workerKey, controlKey.Public.PublicKey).ClaimStart(context.Background(), claim); !errors.Is(err, ErrStartUnavailable) {
		t.Fatalf("request error = %v", err)
	}
	for _, response := range []queue.Message{{Data: []byte("bad")}, {Data: mustStartReply(t, protocol.StartClaimReply{Granted: false, Retry: true, Error: "retry"})}, {Data: mustStartReply(t, protocol.StartClaimReply{Granted: true})}} {
		claimer := NewNATSStartClaimer(&startClaimRequester{response: response}, workerKey, controlKey.Public.PublicKey)
		if err := claimer.ClaimStart(context.Background(), claim); !errors.Is(err, ErrStartUnavailable) {
			t.Fatalf("response %#v error = %v", response, err)
		}
	}
	if err := NewNATSStartClaimer(&startClaimRequester{}, workerKey, controlKey.Public.PublicKey).ClaimStart(context.Background(), protocol.StartClaimPayload{}); !errors.Is(err, ErrStartUnavailable) {
		t.Fatalf("invalid claim error = %v", err)
	}
}

func TestLocalStoreSigningAndConnectionEdges(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/runner.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, found, err := store.LoadConnection(); err != nil || found {
		t.Fatalf("missing connection = %v, %v", found, err)
	}
	key, err := protocol.GenerateSigningKey("runner", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSigningKey(key); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := store.LoadSigningKey()
	if err != nil || !found || loaded.ID != key.ID || !bytes.Equal(loaded.Private, key.Private) {
		t.Fatalf("signing key = %#v, found=%v, err=%v", loaded, found, err)
	}
	if err := store.SaveConnection(RunnerConnection{RunnerID: "runner", MaxMessageBytes: 1}); err != nil {
		t.Fatal(err)
	}
	if connection, found, err := store.LoadConnection(); err != nil || !found || connection.RunnerID != "runner" {
		t.Fatalf("connection = %#v, found=%v, err=%v", connection, found, err)
	}
	for _, value := range []string{`{"id":"runner","private":"not-base64"}`, `{"id":"runner","private":"AQ"}`} {
		if _, err := store.db.Exec(`INSERT INTO messages (id, value) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET value=excluded.value`, "worker.signing_key", []byte(value)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.LoadSigningKey(); err == nil {
			t.Fatalf("invalid signing key accepted: %s", value)
		}
	}
}

func mustStartReply(t *testing.T, reply protocol.StartClaimReply) []byte {
	t.Helper()
	raw, err := protocol.EncodeStartClaimReply(reply)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
