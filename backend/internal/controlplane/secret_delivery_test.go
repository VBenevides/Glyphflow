package controlplane

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type secretDeliveryTestRepository struct {
	record     store.EncryptedSecretRecord
	status     string
	authorized bool
	input      store.SecretRequestInput
}

func (r *secretDeliveryTestRepository) Upsert(_ context.Context, record store.EncryptedSecretRecord) error {
	r.record = record
	return nil
}

func (r *secretDeliveryTestRepository) Find(_ context.Context, id string) (store.EncryptedSecretRecord, bool, error) {
	return r.record, r.record.ID == id, nil
}

func (r *secretDeliveryTestRepository) SetIntegrityStatus(_ context.Context, id, status string, validatedAt time.Time) error {
	if id != r.record.ID {
		return context.Canceled
	}
	r.status = status
	r.record.IntegrityStatus = status
	r.record.LastValidatedAt = &validatedAt
	return nil
}

func (r *secretDeliveryTestRepository) ListStatuses(_ context.Context) ([]store.EncryptedSecretStatusRecord, error) {
	return []store.EncryptedSecretStatusRecord{{ID: r.record.ID, Name: r.record.Name, IntegrityStatus: r.status}}, nil
}

func (r *secretDeliveryTestRepository) AuthorizeSecretRequest(_ context.Context, input store.SecretRequestInput) error {
	r.input = input
	if !r.authorized {
		return context.Canceled
	}
	return nil
}

type secretDeliveryTestKeys struct {
	runnerID string
	keyID    string
	public   ed25519.PublicKey
}

func (r secretDeliveryTestKeys) FindPublicKey(_ context.Context, runnerID, keyID string) (ed25519.PublicKey, error) {
	if runnerID != r.runnerID || keyID != r.keyID {
		return nil, context.Canceled
	}
	return r.public, nil
}

func TestSecretDeliveryResponseAuthenticatesAndAuthorizesValues(t *testing.T) {
	now := time.Now().UTC()
	runnerKey, err := protocol.GenerateSigningKey("runner:runner-1", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	controlKey, err := protocol.GenerateSigningKey("control-plane", now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	encryptionKey := []byte("01234567890123456789012345678901")
	ciphertext, err := platform.EncryptSecret(encryptionKey, "runtime-secret")
	if err != nil {
		t.Fatal(err)
	}
	repository := &secretDeliveryTestRepository{authorized: true, record: store.EncryptedSecretRecord{ID: "secret-1", Name: "Database", EncryptedValue: ciphertext}}
	request := protocol.SecretDeliveryRequest{Version: protocol.ProtocolVersion, RequestID: "attempt-1", OrderID: "attempt-1", RunID: "run-1", Attempt: 1, LeaseToken: "lease-1", RunnerID: "runner-1", RunnerSessionID: "session-1", FencingToken: 2, ExecutionSpecDigest: "digest", SecretRefs: map[string]string{"TOKEN": "secret-1"}, IssuedAt: now}
	payload, err := protocol.EncodeSecretDeliveryRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	envelope := protocol.NewEnvelope(runnerKey.ID, payload)
	if err := envelope.Sign(runnerKey.Private, protocol.SecretDeliverySignatureDomain); err != nil {
		t.Fatal(err)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	responseRaw := secretDeliveryResponse(context.Background(), repository, repository, secretDeliveryTestKeys{runnerID: request.RunnerID, keyID: runnerKey.ID, public: runnerKey.Public.PublicKey}, controlKey, encryptionKey, raw)
	responseEnvelope, err := protocol.DecodeEnvelope(responseRaw)
	if err != nil || responseEnvelope.Verify(controlKey.Public.PublicKey, protocol.SecretDeliverySignatureDomain) != nil {
		t.Fatalf("response envelope = %v", err)
	}
	responsePayload, err := responseEnvelope.PayloadBytes()
	if err != nil {
		t.Fatal(err)
	}
	response, err := protocol.DecodeSecretDeliveryResponse(responsePayload)
	if err != nil || response.Values["TOKEN"] != "runtime-secret" || response.Error != "" {
		t.Fatalf("response = %#v, err = %v", response, err)
	}
	if repository.status != store.SecretIntegrityValid || repository.input.SecretRefs["TOKEN"] != "secret-1" {
		t.Fatalf("repository = %#v", repository)
	}
	if strings.Contains(string(raw), "runtime-secret") {
		t.Fatal("request contained plaintext secret")
	}
}

func TestSecretDeliveryResponseMarksTamperingWithoutReturningValue(t *testing.T) {
	now := time.Now().UTC()
	runnerKey, _ := protocol.GenerateSigningKey("runner:runner-1", now, time.Hour)
	controlKey, _ := protocol.GenerateSigningKey("control-plane", now, time.Hour)
	key := []byte("01234567890123456789012345678901")
	ciphertext, _ := platform.EncryptSecret(key, "runtime-secret")
	ciphertext[len(ciphertext)-1] ^= 1
	repository := &secretDeliveryTestRepository{authorized: true, record: store.EncryptedSecretRecord{ID: "secret-1", EncryptedValue: ciphertext}}
	request := protocol.SecretDeliveryRequest{Version: protocol.ProtocolVersion, RequestID: "attempt-1", OrderID: "attempt-1", RunID: "run-1", Attempt: 1, LeaseToken: "lease-1", RunnerID: "runner-1", RunnerSessionID: "session-1", FencingToken: 2, ExecutionSpecDigest: "digest", SecretRefs: map[string]string{"TOKEN": "secret-1"}, IssuedAt: now}
	payload, _ := protocol.EncodeSecretDeliveryRequest(request)
	envelope := protocol.NewEnvelope(runnerKey.ID, payload)
	_ = envelope.Sign(runnerKey.Private, protocol.SecretDeliverySignatureDomain)
	raw, _ := protocol.EncodeEnvelope(envelope)
	responseRaw := secretDeliveryResponse(context.Background(), repository, repository, secretDeliveryTestKeys{runnerID: "runner-1", keyID: runnerKey.ID, public: runnerKey.Public.PublicKey}, controlKey, key, raw)
	responseEnvelope, _ := protocol.DecodeEnvelope(responseRaw)
	responsePayload, _ := responseEnvelope.PayloadBytes()
	response, err := protocol.DecodeSecretDeliveryResponse(responsePayload)
	if err != nil || response.Error == "" || len(response.Values) != 0 || repository.status != store.SecretIntegrityFailed {
		t.Fatalf("response = %#v, status = %q, err = %v", response, repository.status, err)
	}
	encoded, _ := json.Marshal(response)
	if strings.Contains(string(encoded), "runtime-secret") {
		t.Fatal("error response exposed plaintext secret")
	}
}
