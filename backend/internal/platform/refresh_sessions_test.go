package platform

import (
	"testing"
	"time"
)

func TestRefreshSessionRotationRejectsReplayAndDisabledUsers(t *testing.T) {
	manager := NewRefreshSessionManager()
	sessionID, token, err := manager.Issue("user-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	newID, newToken, err := manager.Rotate(sessionID, token, time.Minute)
	if err != nil || newID == "" || newToken == "" {
		t.Fatal(err)
	}
	if _, _, err := manager.Rotate(sessionID, token, time.Minute); err == nil {
		t.Fatal("replayed refresh token accepted")
	}
	newID, newToken, err = manager.Issue("user-2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	manager.DisableUser("user-2")
	if _, _, err := manager.Rotate(newID, newToken, time.Minute); err == nil {
		t.Fatal("disabled user refresh accepted")
	}
}
