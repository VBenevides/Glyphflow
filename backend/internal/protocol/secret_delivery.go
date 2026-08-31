package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

type SecretDeliveryRequest struct {
	Version             uint8             `json:"version"`
	RequestID           string            `json:"request_id"`
	OrderID             string            `json:"order_id"`
	RunID               string            `json:"run_id"`
	Attempt             uint32            `json:"attempt"`
	LeaseToken          string            `json:"lease_token"`
	RunnerID            string            `json:"runner_id"`
	RunnerSessionID     string            `json:"runner_session_id"`
	FencingToken        uint64            `json:"fencing_token"`
	ExecutionSpecDigest string            `json:"execution_spec_digest"`
	SecretRefs          map[string]string `json:"secret_refs"`
	IssuedAt            time.Time         `json:"issued_at"`
}

type SecretDeliveryResponse struct {
	Version     uint8             `json:"version"`
	RequestID   string            `json:"request_id"`
	Values      map[string]string `json:"values,omitempty"`
	Error       string            `json:"error,omitempty"`
	RespondedAt time.Time         `json:"responded_at"`
}

func (p SecretDeliveryRequest) Validate() error {
	if p.Version != ProtocolVersion || p.RequestID == "" || p.OrderID == "" || p.RunID == "" || p.Attempt == 0 || p.LeaseToken == "" || p.RunnerID == "" || p.RunnerSessionID == "" || p.FencingToken == 0 || p.ExecutionSpecDigest == "" || p.IssuedAt.IsZero() || len(p.SecretRefs) == 0 {
		return errors.New("secret delivery request is incomplete")
	}
	return (OrderPayload{OrderID: p.OrderID, RunID: p.RunID, RunnerID: p.RunnerID, RunnerSessionID: p.RunnerSessionID, LeaseToken: p.LeaseToken, SecretRefs: p.SecretRefs, Command: []string{"secret-delivery"}, WorkingDir: ".", DurationSeconds: 1}).ValidateExecution()
}

func (p SecretDeliveryResponse) Validate() error {
	if p.Version != ProtocolVersion || p.RequestID == "" || p.RespondedAt.IsZero() {
		return errors.New("secret delivery response is incomplete")
	}
	if p.Error != "" && len(p.Values) > 0 {
		return errors.New("secret delivery response contains values with an error")
	}
	if p.Error == "" && len(p.Values) == 0 {
		return errors.New("secret delivery response contains no values")
	}
	return nil
}

func EncodeSecretDeliveryRequest(payload SecretDeliveryRequest) ([]byte, error) {
	return json.Marshal(payload)
}

func DecodeSecretDeliveryRequest(raw []byte) (SecretDeliveryRequest, error) {
	var payload SecretDeliveryRequest
	if err := json.Unmarshal(raw, &payload); err != nil {
		return SecretDeliveryRequest{}, err
	}
	return payload, payload.Validate()
}

func EncodeSecretDeliveryResponse(payload SecretDeliveryResponse) ([]byte, error) {
	return json.Marshal(payload)
}

func DecodeSecretDeliveryResponse(raw []byte) (SecretDeliveryResponse, error) {
	var payload SecretDeliveryResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return SecretDeliveryResponse{}, err
	}
	return payload, payload.Validate()
}
