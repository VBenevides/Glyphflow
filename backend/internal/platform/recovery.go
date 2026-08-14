package platform

import (
	"fmt"
	"strings"
	"sync"
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
	case "completed", "succeeded", "failed", "timed_out", "cancelled", "lost", "runner_lost", "unknown":
		return true
	default:
		return false
	}
}
func TransitionAllowed(from, to string) bool {
	from, to = strings.ToLower(from), strings.ToLower(to)
	if from == to {
		return true
	}
	switch from {
	case "queued", "waiting":
		return to == "dispatch_pending" || to == "dispatched" || to == "cancelled" || to == "failed" || to == "running"
	case "dispatch_pending":
		return to == "dispatched" || to == "cancelled" || to == "failed"
	case "dispatched":
		return to == "accepted" || to == "failed" || to == "cancelled" || to == "runner_lost"
	case "accepted":
		return to == "running" || to == "started" || to == "failed" || to == "cancelled" || to == "unknown"
	case "running", "started":
		return FinalState(to) || to == "retry_wait" || to == "cancelling"
	case "retry_wait":
		return to == "waiting" || to == "failed"
	case "cancelling":
		return to == "completed" || to == "succeeded" || to == "cancelled" || to == "unknown"
	default:
		return false
	}
}

type StateMachine struct {
	mu      sync.Mutex
	state   string
	version uint64
}

func NewStateMachine(initial string) (*StateMachine, error) {
	if strings.TrimSpace(initial) == "" {
		return nil, fmt.Errorf("initial state is required")
	}
	return &StateMachine{state: strings.ToLower(initial)}, nil
}

func (m *StateMachine) Transition(expected, next string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transitionLocked(expected, nil, next)
}

// CompareAndSwap applies a transition only when both the state and its version
// still match the caller's snapshot.
func (m *StateMachine) CompareAndSwap(expected string, expectedVersion uint64, next string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.transitionLocked(expected, &expectedVersion, next)
}

func (m *StateMachine) transitionLocked(expected string, expectedVersion *uint64, next string) error {
	if expected != "" && strings.ToLower(expected) != m.state {
		return fmt.Errorf("state version conflict")
	}
	if expectedVersion != nil && *expectedVersion != m.version {
		return fmt.Errorf("state version conflict")
	}
	next = strings.ToLower(next)
	if !TransitionAllowed(m.state, next) {
		return fmt.Errorf("invalid state transition %s -> %s", m.state, next)
	}
	m.state = next
	m.version++
	return nil
}

func (m *StateMachine) Snapshot() (string, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state, m.version
}
