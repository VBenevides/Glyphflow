package worker

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/protocol"
	"github.com/VBenevides/Glyphflow/backend/internal/queue"
)

var ErrSecretDeliveryUnavailable = errors.New("secret delivery unavailable")

type SecretFetcher interface {
	Fetch(context.Context, protocol.SecretDeliveryRequest) (map[string]string, error)
}

type NATSSecretFetcher struct {
	requester        queue.Requester
	signingKey       protocol.SigningKey
	controlPublicKey ed25519.PublicKey
}

func NewNATSSecretFetcher(requester queue.Requester, signingKey protocol.SigningKey, controlPublicKey ed25519.PublicKey) *NATSSecretFetcher {
	return &NATSSecretFetcher{requester: requester, signingKey: signingKey, controlPublicKey: controlPublicKey}
}

func (f *NATSSecretFetcher) Fetch(ctx context.Context, request protocol.SecretDeliveryRequest) (map[string]string, error) {
	if f == nil || f.requester == nil || len(f.signingKey.Private) != ed25519.PrivateKeySize || len(f.controlPublicKey) != ed25519.PublicKeySize {
		return nil, ErrSecretDeliveryUnavailable
	}
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid request", ErrSecretDeliveryUnavailable)
	}
	payload, err := protocol.EncodeSecretDeliveryRequest(request)
	if err != nil {
		return nil, fmt.Errorf("%w: encode request", ErrSecretDeliveryUnavailable)
	}
	envelope := protocol.NewEnvelope(f.signingKey.ID, payload)
	if err := envelope.Sign(f.signingKey.Private, protocol.SecretDeliverySignatureDomain); err != nil {
		return nil, fmt.Errorf("%w: sign request", ErrSecretDeliveryUnavailable)
	}
	raw, err := protocol.EncodeEnvelope(envelope)
	if err != nil {
		return nil, fmt.Errorf("%w: encode envelope", ErrSecretDeliveryUnavailable)
	}
	response, err := f.requester.Request(ctx, queue.Message{Subject: queue.SecretDeliverySubject(request.RunnerID), Data: raw}, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("%w: request", ErrSecretDeliveryUnavailable)
	}
	responseEnvelope, err := protocol.DecodeEnvelope(response.Data)
	if err != nil || responseEnvelope.Verify(f.controlPublicKey, protocol.SecretDeliverySignatureDomain) != nil {
		return nil, fmt.Errorf("%w: invalid response", ErrSecretDeliveryUnavailable)
	}
	responsePayload, err := responseEnvelope.PayloadBytes()
	if err != nil {
		return nil, fmt.Errorf("%w: response payload", ErrSecretDeliveryUnavailable)
	}
	reply, err := protocol.DecodeSecretDeliveryResponse(responsePayload)
	if err != nil || reply.RequestID != request.RequestID || reply.RespondedAt.After(time.Now().UTC().Add(5*time.Second)) || reply.RespondedAt.Before(time.Now().UTC().Add(-time.Minute)) {
		return nil, fmt.Errorf("%w: response does not match request", ErrSecretDeliveryUnavailable)
	}
	if reply.Error != "" {
		return nil, ErrSecretDeliveryUnavailable
	}
	if len(reply.Values) != len(request.SecretRefs) {
		return nil, fmt.Errorf("%w: response is incomplete", ErrSecretDeliveryUnavailable)
	}
	values := make(map[string]string, len(reply.Values))
	for name, value := range reply.Values {
		if _, ok := request.SecretRefs[name]; !ok {
			clear(values)
			return nil, fmt.Errorf("%w: response contains an unexpected value", ErrSecretDeliveryUnavailable)
		}
		values[name] = value
	}
	return values, nil
}
