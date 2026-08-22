package platform

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

func ScheduleLocation(value string) (*time.Location, error) {
	if len(value) >= 5 && strings.HasPrefix(value, "UTC") && (value[3] == '+' || value[3] == '-') {
		parts := strings.Split(value[4:], ":")
		hours, err := strconv.Atoi(parts[0])
		if err != nil || hours > 23 || len(parts) > 2 {
			return nil, errors.New("UTC offset is invalid")
		}
		minutes := 0
		if len(parts) == 2 {
			minutes, err = strconv.Atoi(parts[1])
			if err != nil || minutes != 0 {
				return nil, errors.New("UTC offset must use whole hours")
			}
		}
		seconds := hours*60*60 + minutes*60
		if value[3] == '-' {
			seconds = -seconds
		}
		return time.FixedZone(value, seconds), nil
	}
	return time.LoadLocation(value)
}
