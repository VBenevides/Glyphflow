package store

import (
	"strings"
	"testing"
)

func TestUpdateTaskRunStateUsesCompareAndSwap(t *testing.T) {
	for _, fragment := range []string{"state_version = state_version + 1", "state_version = $3", "WHERE id = $2"} {
		if !strings.Contains(updateTaskRunStateSQL, fragment) {
			t.Errorf("state update SQL does not contain %q", fragment)
		}
	}
}
