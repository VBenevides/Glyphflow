package protocol

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

const MaxEventErrorBytes = 4096

var secretRefPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

func (p OrderPayload) ValidateExecution() error {
	if p.OrderID == "" || p.RunID == "" || p.RunnerID == "" || p.RunnerSessionID == "" || p.LeaseToken == "" || p.WorkingDir == "" || p.DurationSeconds == 0 || len(p.Command) == 0 {
		return errors.New("order execution fields are required")
	}
	for _, arg := range p.Command {
		if arg == "" {
			return errors.New("order command contains an empty argument")
		}
	}
	for _, ref := range p.SecretRefs {
		if !secretRefPattern.MatchString(ref) || strings.Contains(ref, "..") {
			return errors.New("order secret reference is invalid")
		}
	}
	return nil
}

func (p OrderPayload) ValidateTime(now time.Time, clockTolerance time.Duration) error {
	if clockTolerance < 0 {
		return errors.New("clock tolerance cannot be negative")
	}
	if p.IssuedAt.IsZero() || p.NotBefore.IsZero() || p.ExpiresAt.IsZero() {
		return errors.New("order times are required")
	}
	if p.IssuedAt.After(now.Add(clockTolerance)) {
		return errors.New("order was issued in the future")
	}
	if p.NotBefore.Before(p.IssuedAt.Add(-clockTolerance)) {
		return errors.New("order not-before time precedes issue time")
	}
	if !p.ExpiresAt.After(p.NotBefore) {
		return errors.New("order expiration must follow not-before time")
	}
	if now.Before(p.NotBefore.Add(-clockTolerance)) {
		return errors.New("order is not yet valid")
	}
	if now.After(p.ExpiresAt.Add(clockTolerance)) {
		return errors.New("order has expired")
	}
	return nil
}

func (p EventPayload) ValidateTime(now time.Time, clockTolerance time.Duration) error {
	if clockTolerance < 0 {
		return errors.New("clock tolerance cannot be negative")
	}
	if p.ObservedAt.IsZero() {
		return errors.New("event observation time is required")
	}
	if p.ObservedAt.After(now.Add(clockTolerance)) {
		return errors.New("event observation time is in the future")
	}
	return nil
}

func (p OrderPayload) ValidateIdentity(runnerID, runID string, attempt uint32, leaseToken string) error {
	return validateIdentity(p.RunnerID, p.RunID, p.Attempt, p.LeaseToken, runnerID, runID, attempt, leaseToken)
}

type FreshnessContext struct {
	RunnerID, SessionID, RunID, Recipient, LeaseToken, ExecutionSpecDigest string
	Attempt                                                                uint32
	FencingToken                                                           uint64
	LeaseNotAfter                                                          time.Time
}

func (p OrderPayload) ValidateFreshness(ctx FreshnessContext, now time.Time) error {
	if ctx.RunnerID == "" || ctx.SessionID == "" || ctx.Recipient == "" || ctx.LeaseToken == "" || ctx.ExecutionSpecDigest == "" || ctx.Attempt == 0 || ctx.FencingToken == 0 || ctx.LeaseNotAfter.IsZero() {
		return errors.New("freshness context is incomplete")
	}
	if err := p.ValidateIdentity(ctx.RunnerID, ctx.RunID, ctx.Attempt, ctx.LeaseToken); err != nil {
		return err
	}
	if p.RunnerSessionID != ctx.SessionID || p.Recipient != ctx.Recipient || p.ExecutionSpecDigest != ctx.ExecutionSpecDigest || p.FencingToken != ctx.FencingToken || !p.LeaseNotAfter.Equal(ctx.LeaseNotAfter) {
		return errors.New("order freshness fields do not match")
	}
	if now.After(p.LeaseNotAfter) {
		return errors.New("lease has expired")
	}
	return nil
}

func (p EventPayload) ValidateIdentity(runnerID, runID string, attempt uint32, leaseToken string) error {
	return validateIdentity(p.RunnerID, p.RunID, p.Attempt, p.LeaseToken, runnerID, runID, attempt, leaseToken)
}

func (p EventPayload) ValidateSequence(expected uint64) error {
	if p.Sequence == 0 || expected == 0 {
		return errors.New("event sequence must be greater than zero")
	}
	if p.Sequence != expected {
		return errors.New("unexpected event sequence")
	}
	return nil
}

func (p EventPayload) ValidateError() error {
	if len([]byte(p.Error)) > MaxEventErrorBytes {
		return errors.New("event error exceeds size limit")
	}
	return nil
}

func validateIdentity(actualRunner, actualRun string, actualAttempt uint32, actualLease, expectedRunner, expectedRun string, expectedAttempt uint32, expectedLease string) error {
	if actualRunner == "" || actualRun == "" || actualLease == "" || actualAttempt == 0 {
		return errors.New("message identity fields are required")
	}
	if actualRunner != expectedRunner || actualRun != expectedRun || actualAttempt != expectedAttempt || actualLease != expectedLease {
		return errors.New("message identity does not match expected assignment")
	}
	return nil
}
