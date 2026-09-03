package controlplane

import (
	"context"
	"errors"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/store"
)

type DueRunCreator interface {
	CreateDueRun(context.Context, time.Time, func(store.DueScheduleRecord) (time.Time, error)) (string, bool, error)
}

func RunScheduler(ctx context.Context, schedules DueRunCreator, pollInterval time.Duration) error {
	if schedules == nil || pollInterval <= 0 {
		return errors.New("schedule runner is not configured")
	}
	if err := scheduleDueRuns(ctx, schedules); err != nil {
		return err
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := scheduleDueRuns(ctx, schedules); err != nil {
				return err
			}
		}
	}
}

func scheduleDueRuns(ctx context.Context, schedules DueRunCreator) error {
	now := time.Now().UTC()
	for range 100 {
		_, changed, err := schedules.CreateDueRun(ctx, now, func(schedule store.DueScheduleRecord) (time.Time, error) {
			return NextFire(schedule.Expression, schedule.Timezone, schedule.NextFireAt)
		})
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
	}
	return nil
}
