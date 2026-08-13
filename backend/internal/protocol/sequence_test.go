package protocol

import "testing"

func TestEventSequenceValidation(t *testing.T) {
	event := EventPayload{Sequence: 4}
	if err := event.ValidateSequence(4); err != nil {
		t.Fatal(err)
	}
	if err := event.ValidateSequence(3); err == nil {
		t.Fatal("out-of-order event sequence was accepted")
	}
}
