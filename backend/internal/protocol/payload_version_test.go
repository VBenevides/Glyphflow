package protocol

import (
	"encoding/json"
	"testing"
)

func TestPayloadDecodersRejectUnsupportedVersionsAndTypes(t *testing.T) {
	order, err := json.Marshal(OrderPayload{Version: ProtocolVersion, Type: OrderExecute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeOrderPayload(order); err != nil {
		t.Fatal(err)
	}
	order, err = json.Marshal(OrderPayload{Version: ProtocolVersion + 1, Type: OrderExecute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeOrderPayload(order); err == nil {
		t.Fatal("unsupported order payload version was accepted")
	}
	event, err := json.Marshal(EventPayload{Version: ProtocolVersion, Type: EventType("unknown")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeEventPayload(event); err == nil {
		t.Fatal("unsupported event type was accepted")
	}
}
