package platform

import (
	"fmt"
	"time"
)

type AmbiguityPolicy string

const (
	RetryAmbiguous  AmbiguityPolicy = "RETRY"
	ManualAmbiguous AmbiguityPolicy = "REQUIRE_MANUAL_RECONCILIATION"
	FailedAmbiguous AmbiguityPolicy = "MARK_FAILED"
)

func ResolveAmbiguous(policy AmbiguityPolicy) (string, error) {
	switch policy {
	case RetryAmbiguous:
		return "retry_wait", nil
	case ManualAmbiguous:
		return "unknown", nil
	case FailedAmbiguous:
		return "failed", nil
	default:
		return "", fmt.Errorf("unsupported ambiguity policy %q", policy)
	}
}

func RetryDelay(attempt int, base, max time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for i := 1; i < attempt; i++ {
		if delay >= max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}

type RetryPolicy struct {
	MaxAttempts         int
	RetryableExitCodes  map[int]bool
	RetryableReasons    map[string]bool
	BaseDelay, MaxDelay time.Duration
}

func (p RetryPolicy) Decide(attempt, exitCode int, reason string) (string, time.Duration) {
	if attempt < 1 || p.MaxAttempts < 1 || attempt >= p.MaxAttempts {
		return "failed", 0
	}
	if !p.RetryableExitCodes[exitCode] && !p.RetryableReasons[reason] {
		return "failed", 0
	}
	return "retry_wait", RetryDelay(attempt, p.BaseDelay, p.MaxDelay)
}

type RunAggregator struct {
	Attempts int
	State    string
}

func (r *RunAggregator) Apply(outcome string, retryable bool, maxAttempts int) string {
	r.Attempts++
	switch {
	case outcome == "completed":
		r.State = "completed"
	case outcome == "unknown":
		r.State = "unknown"
	case retryable && r.Attempts < maxAttempts:
		r.State = "retry_wait"
	default:
		r.State = "failed"
	}
	return r.State
}
func FinalState(state string) bool {
	switch state {
	case "completed", "failed", "timed_out", "cancelled", "lost", "unknown":
		return true
	default:
		return false
	}
}
func TransitionAllowed(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case "queued", "waiting":
		return to == "dispatched" || to == "cancelled" || to == "failed" || to == "running"
	case "dispatched":
		return to == "accepted" || to == "failed" || to == "cancelled"
	case "accepted":
		return to == "running" || to == "failed" || to == "cancelled"
	case "running", "started":
		return FinalState(to) || to == "retry_wait" || to == "cancelling"
	case "retry_wait":
		return to == "waiting" || to == "failed"
	case "cancelling":
		return to == "completed" || to == "cancelled" || to == "unknown"
	default:
		return false
	}
}
