package protocol

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

const ProtocolVersion uint8 = 1

type Envelope struct {
	Version   uint8  `json:"version"`
	KeyID     string `json:"key_id"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

func NewEnvelope(keyID string, payload []byte) Envelope {
	return Envelope{
		Version: ProtocolVersion,
		KeyID:   keyID,
		Payload: base64.StdEncoding.EncodeToString(payload),
	}
}

func (e Envelope) PayloadBytes() ([]byte, error) {
	if e.Payload == "" {
		return nil, errors.New("envelope payload is empty")
	}
	return base64.StdEncoding.DecodeString(e.Payload)
}

func (e Envelope) SignatureBytes() ([]byte, error) {
	if e.Signature == "" {
		return nil, errors.New("envelope signature is empty")
	}
	return base64.StdEncoding.DecodeString(e.Signature)
}

func EncodeEnvelope(e Envelope) ([]byte, error) {
	return json.Marshal(e)
}

func encodeBase64(value []byte) string {
	return base64.StdEncoding.EncodeToString(value)
}

func DecodeEnvelope(raw []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Envelope{}, err
	}
	if envelope.KeyID == "" {
		return Envelope{}, errors.New("envelope key_id is empty")
	}
	if _, err := envelope.PayloadBytes(); err != nil {
		return Envelope{}, err
	}
	if envelope.Signature != "" {
		if _, err := envelope.SignatureBytes(); err != nil {
			return Envelope{}, err
		}
	}
	return envelope, nil
}
