package protocol

import "time"

type ReplayAcceptor interface {
	Accept(string) error
}

func VerifyOrder(raw []byte, keys Keyring, now time.Time, runnerID, runID string, attempt uint32, leaseToken string, tolerance time.Duration, replay ReplayAcceptor) (OrderPayload, error) {
	envelope, err := DecodeEnvelope(raw)
	if err != nil {
		return OrderPayload{}, err
	}
	if err := keys.VerifyAt(envelope, OrderSignatureDomain, now); err != nil {
		return OrderPayload{}, err
	}
	payload, err := DecodeOrderPayload(mustPayload(envelope))
	if err != nil {
		return OrderPayload{}, err
	}
	if err := payload.ValidateTime(now, tolerance); err != nil {
		return OrderPayload{}, err
	}
	if err := payload.ValidateIdentity(runnerID, runID, attempt, leaseToken); err != nil {
		return OrderPayload{}, err
	}
	if replay != nil {
		if err := replay.Accept(payload.OrderID); err != nil {
			return OrderPayload{}, err
		}
	}
	return payload, nil
}

func VerifyEvent(raw []byte, keys Keyring, now time.Time, runnerID, runID string, attempt uint32, leaseToken string, expectedSequence uint64, tolerance time.Duration, replay ReplayAcceptor) (EventPayload, error) {
	envelope, err := DecodeEnvelope(raw)
	if err != nil {
		return EventPayload{}, err
	}
	if err := keys.VerifyAt(envelope, EventSignatureDomain, now); err != nil {
		return EventPayload{}, err
	}
	payload, err := DecodeEventPayload(mustPayload(envelope))
	if err != nil {
		return EventPayload{}, err
	}
	if err := payload.ValidateTime(now, tolerance); err != nil {
		return EventPayload{}, err
	}
	if err := payload.ValidateIdentity(runnerID, runID, attempt, leaseToken); err != nil {
		return EventPayload{}, err
	}
	if err := payload.ValidateSequence(expectedSequence); err != nil {
		return EventPayload{}, err
	}
	if err := payload.ValidateError(); err != nil {
		return EventPayload{}, err
	}
	if replay != nil {
		if err := replay.Accept(payload.EventID); err != nil {
			return EventPayload{}, err
		}
	}
	return payload, nil
}

func mustPayload(envelope Envelope) []byte {
	payload, _ := envelope.PayloadBytes()
	return payload
}
