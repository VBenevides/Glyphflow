package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
)

// payloadMagic identifies the versioned Glyphflow payload wire format.
// The JSON bytes inside the frame use encoding/json's deterministic field and
// map ordering. The frame makes the signed payload binary and length bounded.
var payloadMagic = [5]byte{'G', 'F', 'P', 1, 0}

const maxPayloadFrameBytes = DefaultMaxEnvelopeBytes

func encodePayloadFrame(value any) ([]byte, error) {
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(canonical) > maxPayloadFrameBytes {
		return nil, errors.New("payload exceeds size limit")
	}
	frame := bytes.NewBuffer(make([]byte, 0, len(payloadMagic)+4+len(canonical)))
	_, _ = frame.Write(payloadMagic[:])
	if err := binary.Write(frame, binary.BigEndian, uint32(len(canonical))); err != nil {
		return nil, err
	}
	_, _ = frame.Write(canonical)
	return frame.Bytes(), nil
}

func decodePayloadFrame(raw []byte) ([]byte, error) {
	if len(raw) < len(payloadMagic)+4 || !bytes.Equal(raw[:len(payloadMagic)], payloadMagic[:]) {
		return raw, nil
	}
	length := binary.BigEndian.Uint32(raw[len(payloadMagic) : len(payloadMagic)+4])
	if length > maxPayloadFrameBytes || int(length) != len(raw)-len(payloadMagic)-4 {
		return nil, errors.New("invalid payload frame length")
	}
	return raw[len(payloadMagic)+4:], nil
}

// EncodeOrderPayload returns the canonical binary payload signed in an order.
func EncodeOrderPayload(payload OrderPayload) ([]byte, error) {
	return encodePayloadFrame(payload)
}

// EncodeEventPayload returns the canonical binary payload signed in an event.
func EncodeEventPayload(payload EventPayload) ([]byte, error) {
	return encodePayloadFrame(payload)
}
