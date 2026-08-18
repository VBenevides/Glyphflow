package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

type StartClaimPayload struct {
	Version             uint8     `json:"version"`
	RequestID           string    `json:"request_id"`
	RunID               string    `json:"run_id"`
	RunnerID            string    `json:"runner_id"`
	RunnerSessionID     string    `json:"runner_session_id"`
	LeaseToken          string    `json:"lease_token"`
	Attempt             uint32    `json:"attempt"`
	FencingToken        uint64    `json:"fencing_token"`
	ExecutionSpecDigest string    `json:"execution_spec_digest"`
	IssuedAt            time.Time `json:"issued_at"`
}

type StartGrantPayload struct {
	StartClaimPayload
	GrantedAt time.Time `json:"granted_at"`
}

type StartClaimReply struct {
	Granted bool      `json:"granted"`
	Retry   bool      `json:"retry,omitempty"`
	Error   string    `json:"error,omitempty"`
	Grant   *Envelope `json:"grant,omitempty"`
}

func (p StartClaimPayload) Validate() error {
	if p.Version != ProtocolVersion || p.RequestID == "" || p.RunID == "" || p.RunnerID == "" || p.RunnerSessionID == "" || p.LeaseToken == "" || p.Attempt == 0 || p.FencingToken == 0 || p.ExecutionSpecDigest == "" || p.IssuedAt.IsZero() {
		return errors.New("start claim is incomplete")
	}
	return nil
}

func EncodeStartClaimReply(reply StartClaimReply) ([]byte, error) {
	return json.Marshal(reply)
}

func DecodeStartClaimReply(raw []byte) (StartClaimReply, error) {
	var reply StartClaimReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return StartClaimReply{}, err
	}
	return reply, nil
}
