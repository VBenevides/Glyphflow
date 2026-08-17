package platform

import "errors"

type Cancellation struct {
	RunID      string
	AttemptID  string
	SessionID  string
	LeaseToken string
	Fencing    uint64
}

func ValidateCancellation(c Cancellation, active Cancellation) error {
	if c.RunID == "" || c.AttemptID == "" || c.SessionID == "" || c.LeaseToken == "" || c.Fencing == 0 {
		return errors.New("cancellation identity is incomplete")
	}
	if c != active {
		return errors.New("cancellation does not match active attempt")
	}
	return nil
}

func ApplyCancellation(c Cancellation, active Cancellation, currentState string, processCompleted bool) (string, error) {
	if err := ValidateCancellation(c, active); err != nil {
		return "", err
	}
	if processCompleted || currentState == "completed" || currentState == "succeeded" {
		return "completed", nil
	}
	switch currentState {
	case "waiting", "queued", "dispatched", "accepted":
		return "cancelled", nil
	case "running", "started":
		return "cancelling", nil
	case "cancelling":
		return "cancelled", nil
	default:
		return "", errors.New("attempt is not cancellable")
	}
}
