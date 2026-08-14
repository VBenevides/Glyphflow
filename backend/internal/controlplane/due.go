package controlplane

import (
	"sync"
	"time"
)

type DueSchedule struct {
	ID           string
	NextFire     time.Time
	Interval     time.Duration
	OccurrenceAt time.Time
}

type DueScheduleQueue struct {
	mu    sync.Mutex
	items map[string]DueSchedule
}

func NewDueScheduleQueue() *DueScheduleQueue {
	return &DueScheduleQueue{items: make(map[string]DueSchedule)}
}

func (q *DueScheduleQueue) Add(schedule DueSchedule) {
	if schedule.ID == "" || schedule.Interval <= 0 {
		return
	}
	q.mu.Lock()
	q.items[schedule.ID] = schedule
	q.mu.Unlock()
}

func (q *DueScheduleQueue) Claim(now time.Time) (DueSchedule, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for id, schedule := range q.items {
		if !schedule.NextFire.After(now) {
			claimed := schedule
			claimed.OccurrenceAt = schedule.NextFire
			for !schedule.NextFire.After(now) {
				schedule.NextFire = schedule.NextFire.Add(schedule.Interval)
			}
			q.items[id] = schedule
			schedule.OccurrenceAt = claimed.OccurrenceAt
			return schedule, true
		}
	}
	return DueSchedule{}, false
}
