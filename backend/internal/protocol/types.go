package protocol

const (
	OrderExecute OrderType = "execute"
	OrderCancel  OrderType = "cancel"

	EventAccepted  EventType = "accepted"
	EventRejected  EventType = "rejected"
	EventStarted   EventType = "started"
	EventHeartbeat EventType = "heartbeat"
	EventCompleted EventType = "completed"
	EventFailed    EventType = "failed"
	EventTimedOut  EventType = "timed_out"
	EventCancelled EventType = "cancelled"
)

func (t OrderType) Valid() bool {
	return t == OrderExecute || t == OrderCancel
}

func (t EventType) Valid() bool {
	switch t {
	case EventAccepted, EventRejected, EventStarted, EventHeartbeat, EventCompleted, EventFailed, EventTimedOut, EventCancelled:
		return true
	default:
		return false
	}
}
