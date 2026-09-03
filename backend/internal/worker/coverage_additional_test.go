package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

type coverageRequester struct {
	response queue.Message
	err      error
}

func (r coverageRequester) Request(context.Context, queue.Message, time.Duration) (queue.Message, error) {
	return r.response, r.err
}

func TestNATSSecretFetcherCoverage(t *testing.T) {
	runnerKey, err := protocol.GenerateSigningKey("runner", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	controlKey, err := protocol.GenerateSigningKey("control", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.SecretDeliveryRequest{Version: protocol.ProtocolVersion, RequestID: "request", OrderID: "order", RunID: "run", Attempt: 1, LeaseToken: "lease", RunnerID: "runner", RunnerSessionID: "session", FencingToken: 1, ExecutionSpecDigest: "digest", SecretRefs: map[string]string{"TOKEN": "secret"}, IssuedAt: time.Now().UTC()}
	responsePayload, err := protocol.EncodeSecretDeliveryResponse(protocol.SecretDeliveryResponse{Version: protocol.ProtocolVersion, RequestID: request.RequestID, Values: map[string]string{"TOKEN": "value"}, RespondedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	responseEnvelope := protocol.NewEnvelope(controlKey.ID, responsePayload)
	if err := responseEnvelope.Sign(controlKey.Private, protocol.SecretDeliverySignatureDomain); err != nil {
		t.Fatal(err)
	}
	responseRaw, err := protocol.EncodeEnvelope(responseEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	fetcher := NewNATSSecretFetcher(coverageRequester{response: queue.Message{Data: responseRaw}}, runnerKey, controlKey.Public.PublicKey)
	values, err := fetcher.Fetch(context.Background(), request)
	if err != nil || values["TOKEN"] != "value" {
		t.Fatalf("values = %#v err=%v", values, err)
	}
	if _, err := (*NATSSecretFetcher)(nil).Fetch(context.Background(), request); !errors.Is(err, ErrSecretDeliveryUnavailable) {
		t.Fatalf("nil fetcher error = %v", err)
	}
	if _, err := NewNATSSecretFetcher(coverageRequester{err: errors.New("offline")}, runnerKey, controlKey.Public.PublicKey).Fetch(context.Background(), request); !errors.Is(err, ErrSecretDeliveryUnavailable) {
		t.Fatalf("request failure = %v", err)
	}
	request.SecretRefs = nil
	if _, err := fetcher.Fetch(context.Background(), request); err == nil {
		t.Fatal("invalid secret request accepted")
	}
}
