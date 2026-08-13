package controlplane

import (
	"errors"
	"time"
)

type Schedule struct {
	Manual   bool
	At       *time.Time
	Cron     string
	Timezone string
}

func (s Schedule) Next(now time.Time) (time.Time, error) {
	if s.Manual {
		return now, nil
	}
	location := time.UTC
	if s.Timezone != "" {
		var err error
		location, err = time.LoadLocation(s.Timezone)
		if err != nil {
			return time.Time{}, err
		}
	}
	if s.At != nil {
		at := s.At.In(location)
		if at.After(now.In(location)) {
			return at, nil
		}
		return time.Time{}, errors.New("fixed schedule is in the past")
	}
	if len(splitFields(s.Cron)) == 5 {
		return nextCronMinute(now.In(location), s.Cron)
	}
	return time.Time{}, errors.New("schedule must be manual, fixed-time, or five-field cron")
}

func nextCronMinute(now time.Time, expression string) (time.Time, error) {
	fields := splitFields(expression)
	if len(fields) != 5 {
		return time.Time{}, errors.New("cron requires five fields")
	}
	for i := 1; i <= 24*60*370; i++ {
		candidate := now.Truncate(time.Minute).Add(time.Duration(i) * time.Minute)
		if matches(fields[0], candidate.Minute()) && matches(fields[1], candidate.Hour()) && matches(fields[2], candidate.Day()) && matches(fields[3], int(candidate.Month())) && matches(fields[4], int(candidate.Weekday())) {
			return candidate, nil
		}
	}
	return time.Time{}, errors.New("cron has no occurrence in search window")
}

func splitFields(value string) []string {
	var fields []string
	for _, field := range []byte(value) {
		if field == ' ' {
			continue
		}
	}
	start := 0
	for i := 0; i <= len(value); i++ {
		if i == len(value) || value[i] == ' ' {
			if i > start {
				fields = append(fields, value[start:i])
			}
			start = i + 1
		}
	}
	return fields
}

func matches(field string, value int) bool {
	if field == "*" {
		return true
	}
	for _, part := range splitComma(field) {
		if part == "*" {
			return true
		}
		var n int
		if _, err := fmtSscanf(part, &n); err == nil && n == value {
			return true
		}
	}
	return false
}

func splitComma(value string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(value); i++ {
		if i == len(value) || value[i] == ',' {
			parts = append(parts, value[start:i])
			start = i + 1
		}
	}
	return parts
}

func fmtSscanf(value string, target *int) (int, error) {
	var parsed int
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, errors.New("not an integer")
		}
		parsed = parsed*10 + int(r-'0')
	}
	*target = parsed
	return 1, nil
}
