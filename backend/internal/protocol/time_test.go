package protocol

import (
	"testing"
	"time"
)

func TestOrderTimeValidation(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	valid := OrderPayload{
		IssuedAt:  now.Add(-time.Minute),
		NotBefore: now.Add(-time.Second),
		ExpiresAt: now.Add(time.Minute),
	}
	if err := valid.ValidateTime(now, time.Second); err != nil {
		t.Fatal(err)
	}
	valid.ExpiresAt = now.Add(-time.Minute)
	if err := valid.ValidateTime(now, time.Second); err == nil {
		t.Fatal("expired order was accepted")
	}
}

func TestEventTimeValidationRejectsFutureObservation(t *testing.T) {
	now := time.Unix(100, 0).UTC()
	event := EventPayload{ObservedAt: now.Add(2 * time.Second)}
	if err := event.ValidateTime(now, time.Second); err == nil {
		t.Fatal("future event observation was accepted")
	}
}
