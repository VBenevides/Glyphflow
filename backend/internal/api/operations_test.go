package api

import (
	"testing"
	"time"
)

func TestPreviewOccurrencesReturnsFiveIncreasingTimes(t *testing.T) {
	items, err := previewOccurrences("*/5 * * * *", "UTC", time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC))
	if err != nil || len(items) != 5 {
		t.Fatalf("preview = %#v, err = %v", items, err)
	}
	for i := 1; i < len(items); i++ {
		if items[i] <= items[i-1] {
			t.Fatalf("preview is not increasing: %#v", items)
		}
	}
}
