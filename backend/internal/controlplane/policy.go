package controlplane

import (
	"errors"
	"time"
)

type MisfirePolicy string
type ConcurrencyPolicy string

const (
	MisfireSkipAll      MisfirePolicy     = "SKIP_ALL"
	MisfireRunLatest    MisfirePolicy     = "RUN_LATEST"
	MisfireRunAll       MisfirePolicy     = "RUN_ALL"
	MisfireRunUpToN     MisfirePolicy     = "RUN_UP_TO_N"
	MisfireFailAndAlert MisfirePolicy     = "FAIL_AND_ALERT"
	ConcurrencyQueue    ConcurrencyPolicy = "QUEUE"
	ConcurrencySkip     ConcurrencyPolicy = "SKIP"
	ConcurrencyReplace  ConcurrencyPolicy = "REPLACE"
	ConcurrencyAllow    ConcurrencyPolicy = "ALLOW"
)

type SchedulePolicy struct {
	Misfire           MisfirePolicy
	Concurrency       ConcurrencyPolicy
	MaxConcurrentRuns int
	StartDeadline     time.Duration
	ExecutionTimeout  time.Duration
}

func (p SchedulePolicy) Validate() error {
	switch p.Misfire {
	case MisfireSkipAll, MisfireRunLatest, MisfireRunAll, MisfireRunUpToN, MisfireFailAndAlert:
	default:
		return errors.New("unsupported misfire policy")
	}
	switch p.Concurrency {
	case ConcurrencyQueue, ConcurrencySkip, ConcurrencyReplace:
		if p.MaxConcurrentRuns != 0 {
			return errors.New("max concurrent runs only applies to ALLOW")
		}
	case ConcurrencyAllow:
		if p.MaxConcurrentRuns < 1 {
			return errors.New("ALLOW requires a positive maximum")
		}
	default:
		return errors.New("unsupported concurrency policy")
	}
	if p.StartDeadline < 0 || p.ExecutionTimeout <= 0 {
		return errors.New("invalid schedule deadlines")
	}
	return nil
}

func (p SchedulePolicy) OccurrencesDue(now, next time.Time, interval time.Duration, active int) (int, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}
	if interval <= 0 || !next.Before(now) {
		return 0, nil
	}
	if p.Concurrency == ConcurrencySkip && active > 0 {
		return 0, nil
	}
	if p.Concurrency == ConcurrencyReplace && active > 0 {
		return 1, nil
	}
	count := int(now.Sub(next)/interval) + 1
	switch p.Misfire {
	case MisfireSkipAll:
		return 1, nil
	case MisfireRunLatest:
		return 1, nil
	case MisfireRunUpToN:
		if p.MaxConcurrentRuns > 0 && count > p.MaxConcurrentRuns {
			return p.MaxConcurrentRuns, nil
		}
	}
	return count, nil
}
