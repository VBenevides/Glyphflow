package platform

import (
	"crypto/subtle"
	"errors"
	"time"
)

type Enrollment struct {
	RunnerID  string
	TokenHash string
	ExpiresAt time.Time
	Used      bool
}

func ClaimEnrollment(enrollment *Enrollment, token, runnerID string, now time.Time) error {
	if enrollment == nil || enrollment.RunnerID != runnerID {
		return errors.New("enrollment runner does not match")
	}
	if !TokenUsable(enrollment.ExpiresAt, enrollment.Used, now) {
		return errors.New("enrollment token is expired or used")
	}
	provided, expected := []byte(HashToken(token)), []byte(enrollment.TokenHash)
	if len(provided) != len(expected) || subtle.ConstantTimeCompare(provided, expected) != 1 {
		return errors.New("enrollment token is invalid")
	}
	enrollment.Used = true
	return nil
}
