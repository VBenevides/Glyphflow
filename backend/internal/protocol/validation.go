package protocol

import (
	"errors"
	"time"
)

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

func (p EventPayload) ValidateIdentity(runnerID, runID string, attempt uint32, leaseToken string) error {
	return validateIdentity(p.RunnerID, p.RunID, p.Attempt, p.LeaseToken, runnerID, runID, attempt, leaseToken)
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
