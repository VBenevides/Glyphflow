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
	CatchUpLimit      int
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
	if p.StartDeadline < 0 || p.ExecutionTimeout <= 0 || p.CatchUpLimit < 0 {
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
		limit := p.CatchUpLimit
		if limit == 0 {
			limit = p.MaxConcurrentRuns
		}
		if limit > 0 && count > limit {
			return limit, nil
		}
	}
	return count, nil
}

type ScheduleDecision struct {
	Occurrences int
	Skipped     bool
	Failed      bool
	Replaced    bool
	Deadline    time.Time
}

func (p SchedulePolicy) Evaluate(now, next time.Time, interval time.Duration, active int) (ScheduleDecision, error) {
	occurrences, err := p.OccurrencesDue(now, next, interval, active)
	if err != nil {
		return ScheduleDecision{}, err
	}
	decision := ScheduleDecision{Occurrences: occurrences}
	if p.Concurrency == ConcurrencySkip && active > 0 {
		decision.Skipped = true
	}
	if p.Concurrency == ConcurrencyReplace && active > 0 {
		decision.Replaced = true
	}
	if p.Misfire == MisfireFailAndAlert && occurrences > 0 {
		decision.Failed = true
		decision.Occurrences = 0
	}
	if p.StartDeadline > 0 {
		decision.Deadline = next.Add(p.StartDeadline)
	}
	return decision, nil
}
func (p SchedulePolicy) DeadlineExceeded(scheduled, started time.Time) bool {
	return p.StartDeadline > 0 && started.After(scheduled.Add(p.StartDeadline))
}
func (p SchedulePolicy) AllowsConcurrency(active int) bool {
	switch p.Concurrency {
	case ConcurrencyQueue:
		return active == 0
	case ConcurrencySkip:
		return active == 0
	case ConcurrencyReplace:
		return true
	case ConcurrencyAllow:
		return active < p.MaxConcurrentRuns
	default:
		return false
	}
}
