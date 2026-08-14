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
