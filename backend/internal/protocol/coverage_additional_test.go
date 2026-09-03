package protocol

import (
	"bytes"
	"testing"
	"time"
)

func validSecretDeliveryRequest() SecretDeliveryRequest {
	return SecretDeliveryRequest{
		Version: ProtocolVersion, RequestID: "request", OrderID: "order", RunID: "run", Attempt: 1,
		LeaseToken: "lease", RunnerID: "runner", RunnerSessionID: "session", FencingToken: 1,
		ExecutionSpecDigest: "digest", SecretRefs: map[string]string{"TOKEN": "secret"}, IssuedAt: time.Now().UTC(),
	}
}

func TestPayloadFramesAndDeliveryContracts(t *testing.T) {
	encoded, err := EncodeOrderPayload(OrderPayload{OrderID: "order", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", Command: []string{"echo"}, WorkingDir: ".", DurationSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodePayloadFrame(encoded); err != nil || !bytes.Equal(decoded, encoded[len(payloadMagic)+4:]) {
		t.Fatalf("decoded frame = %q, err = %v", decoded, err)
	}
	if raw, err := decodePayloadFrame([]byte("plain")); err != nil || string(raw) != "plain" {
		t.Fatalf("plain frame = %q, err = %v", raw, err)
	}
	bad := append([]byte(nil), encoded...)
	bad[len(payloadMagic)+3]++
	if _, err := decodePayloadFrame(bad); err == nil {
		t.Fatal("invalid frame length accepted")
	}

	request := validSecretDeliveryRequest()
	raw, err := EncodeSecretDeliveryRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := DecodeSecretDeliveryRequest(raw); err != nil || decoded.RequestID != request.RequestID {
		t.Fatalf("request round trip = %#v, err = %v", decoded, err)
	}
	response := SecretDeliveryResponse{Version: ProtocolVersion, RequestID: request.RequestID, Values: map[string]string{"TOKEN": "value"}, RespondedAt: time.Now().UTC()}
	raw, err = EncodeSecretDeliveryResponse(response)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSecretDeliveryResponse(raw); err != nil {
		t.Fatal(err)
	}
	response.Error = "failed"
	response.Values = map[string]string{"TOKEN": "value"}
	if err := response.Validate(); err == nil {
		t.Fatal("response with error and values accepted")
	}
}

func TestSigningAndStartClaimContracts(t *testing.T) {
	if _, err := GenerateSigningKey("", time.Now(), time.Hour); err == nil {
		t.Fatal("empty signing key accepted")
	}
	key, err := GenerateSigningKey("key", time.Now().UTC(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := key.SignEvent([]byte("event")); err != nil {
		t.Fatal(err)
	}
	claim := StartClaimPayload{Version: ProtocolVersion, RequestID: "request", RunID: "run", RunnerID: "runner", RunnerSessionID: "session", LeaseToken: "lease", Attempt: 1, FencingToken: 1, ExecutionSpecDigest: "digest", IssuedAt: time.Now().UTC()}
	if err := claim.Validate(); err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeStartClaimReply(StartClaimReply{Granted: true})
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := DecodeStartClaimReply(raw); err != nil || !reply.Granted {
		t.Fatalf("start reply = %#v, err = %v", reply, err)
	}
}
