package platform

import "time"

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
