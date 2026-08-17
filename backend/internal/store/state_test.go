package store

import (
	"strings"
	"testing"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

func TestUpdateTaskRunStateUsesCompareAndSwap(t *testing.T) {
	for _, fragment := range []string{"state_version = state_version + 1", "state = $3", "state_version = $5", "WHERE id = $2"} {
		if !strings.Contains(updateTaskRunStateSQL, fragment) {
			t.Errorf("state update SQL does not contain %q", fragment)
		}
	}
}

func TestInvalidTransitionIsRejectedBeforeDatabaseUse(t *testing.T) {
	if platform.TransitionAllowed("completed", "running") {
		t.Fatal("invalid transition was accepted")
	}
}
