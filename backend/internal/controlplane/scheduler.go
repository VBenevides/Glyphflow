package controlplane

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

type Schedule struct {
	Manual   bool
	At       *time.Time
	Cron     string
	Timezone string
}

type cronCalendar struct {
	dom, month, dow map[int]bool
	domAny, dowAny  bool
}

func NextFire(expression, timezone string, now time.Time) (time.Time, error) {
	return (Schedule{Cron: expression, Timezone: timezone}).Next(now)
}

func (s Schedule) Next(now time.Time) (time.Time, error) {
	if s.Manual {
		return now, nil
	}
	location := time.UTC
	if s.Timezone != "" {
		var err error
		location, err = platform.ScheduleLocation(s.Timezone)
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
	minute, hour, dom, month, dow, err := parseCronFields(expression)
	if err != nil {
		return time.Time{}, err
	}
	domAny := cronFieldIsAny(dom, 1, 31)
	dowAny := cronFieldIsAny(dow, 0, 6)
	if dowAny && !cronHasCalendarDate(dom, month) {
		return time.Time{}, errors.New("cron has no occurrence in search window")
	}
	for i := 1; i <= 24*60*366*8; i++ {
		candidate := now.Truncate(time.Minute).Add(time.Duration(i) * time.Minute)
		if cronMinuteMatches(candidate, minute, hour, cronCalendar{dom: dom, month: month, dow: dow, domAny: domAny, dowAny: dowAny}) {
			return candidate, nil
		}
	}
	return time.Time{}, errors.New("cron has no occurrence in search window")
}

func parseCronFields(expression string) (map[int]bool, map[int]bool, map[int]bool, map[int]bool, map[int]bool, error) {
	fields := splitFields(expression)
	if len(fields) != 5 {
		return nil, nil, nil, nil, nil, errors.New("cron requires five fields")
	}
	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	month, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	dow, err := parseCronField(fields[4], 0, 6)
	return minute, hour, dom, month, dow, err
}

func cronMinuteMatches(candidate time.Time, minute, hour map[int]bool, calendar cronCalendar) bool {
	if !minute[candidate.Minute()] || !hour[candidate.Hour()] || !calendar.month[int(candidate.Month())] {
		return false
	}
	if calendar.domAny && calendar.dowAny {
		return true
	}
	if calendar.domAny {
		return calendar.dow[int(candidate.Weekday())]
	}
	if calendar.dowAny {
		return calendar.dom[candidate.Day()]
	}
	return calendar.dom[candidate.Day()] || calendar.dow[int(candidate.Weekday())]
}

func cronFieldIsAny(values map[int]bool, min, max int) bool {
	return len(values) == max-min+1
}

func cronHasCalendarDate(dom, months map[int]bool) bool {
	for month := range months {
		maxDay := 31
		switch month {
		case 2:
			maxDay = 29
		case 4, 6, 9, 11:
			maxDay = 30
		}
		for day := range dom {
			if day <= maxDay {
				return true
			}
		}
	}
	return false
}

func splitFields(value string) []string {
	return strings.Fields(value)
}

func parseCronField(field string, min, max int) (map[int]bool, error) {
	values := make(map[int]bool)
	for _, part := range splitComma(field) {
		if part == "" {
			return nil, errors.New("cron contains an empty field")
		}
		start, end, step, err := parseCronPart(part, min, max)
		if err != nil {
			return nil, err
		}
		if start < min || end > max {
			return nil, errors.New("cron value is outside its field range")
		}
		for value := start; value <= end; value += step {
			values[value] = true
		}
	}
	return values, nil
}

func parseCronPart(part string, min, max int) (int, int, int, error) {
	base, step, hasStep, err := parseCronStep(part)
	if err != nil {
		return 0, 0, 0, err
	}
	start, end, err := parseCronRange(base, min, max, hasStep)
	return start, end, step, err
}

func parseCronStep(part string) (string, int, bool, error) {
	slash := strings.IndexByte(part, '/')
	if slash < 0 {
		return part, 1, false, nil
	}
	step, err := strconv.Atoi(part[slash+1:])
	if err != nil || step <= 0 {
		return "", 0, false, errors.New("cron step must be positive")
	}
	return part[:slash], step, true, nil
}

func parseCronRange(base string, min, max int, hasStep bool) (int, int, error) {
	start, end := min, max
	if base == "*" {
		return start, end, nil
	}
	dash := strings.IndexByte(base, '-')
	if dash >= 0 {
		start, err := strconv.Atoi(base[:dash])
		if err != nil {
			return 0, 0, errors.New("cron range is invalid")
		}
		end, err = strconv.Atoi(base[dash+1:])
		if err != nil || end < start {
			return 0, 0, errors.New("cron range is invalid")
		}
		return start, end, nil
	}
	var err error
	start, err = strconv.Atoi(base)
	if err != nil {
		return 0, 0, errors.New("cron value is invalid")
	}
	if !hasStep {
		end = start
	}
	return start, end, nil
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
