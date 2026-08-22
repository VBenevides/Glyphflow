package platform

import (
	"testing"
	"time"
)

func TestScheduleLocationSemantics(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		offset int
	}{
		{name: "UTC", value: "UTC", offset: 0},
		{name: "positive whole hour", value: "UTC+23:00", offset: 23 * 60 * 60},
		{name: "negative whole hour", value: "UTC-05:00", offset: -5 * 60 * 60},
		{name: "IANA zone", value: "America/Sao_Paulo", offset: -3 * 60 * 60},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			location, err := ScheduleLocation(test.value)
			if err != nil {
				t.Fatal(err)
			}
			_, offset := time.Now().In(location).Zone()
			if offset != test.offset {
				t.Fatalf("offset = %d, want %d", offset, test.offset)
			}
		})
	}
	for _, value := range []string{"UTC+01:30", "UTC+24:00", "Not/AZone"} {
		if _, err := ScheduleLocation(value); err == nil {
			t.Fatalf("ScheduleLocation(%q) accepted invalid timezone", value)
		}
	}
}
