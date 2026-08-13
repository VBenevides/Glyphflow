package protocol

import "testing"

func TestEnvelopeRoundTrip(t *testing.T) {
	wantPayload := []byte(`{"run_id":"run-1"}`)
	want := NewEnvelope("control-plane-1", wantPayload)
	raw, err := EncodeEnvelope(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeEnvelope(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.KeyID != want.KeyID || got.Version != ProtocolVersion {
		t.Fatalf("envelope metadata mismatch: %#v", got)
	}
	payload, err := got.PayloadBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != string(wantPayload) {
		t.Fatalf("payload mismatch: %q", payload)
	}
}
