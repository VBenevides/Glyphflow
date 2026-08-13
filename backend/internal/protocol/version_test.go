package protocol

import "testing"

func TestDecodeEnvelopeRejectsUnsupportedVersion(t *testing.T) {
	envelope := NewEnvelope("control-plane-1", []byte("payload"))
	envelope.Version = ProtocolVersion + 1
	raw, err := EncodeEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeEnvelope(raw); err == nil {
		t.Fatal("unsupported protocol version was accepted")
	}
}
