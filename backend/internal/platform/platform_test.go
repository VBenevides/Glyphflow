package platform

import (
	"strings"
	"testing"
	"time"
)

func TestEnrollmentRedactionAndPaths(t *testing.T) {
	plain, hash, err := NewEnrollmentToken(32)
	if err != nil || hash != HashToken(plain) {
		t.Fatal("token hashing failed")
	}
	if !TokenUsable(time.Now().Add(time.Minute), false, time.Now()) {
		t.Fatal("valid token rejected")
	}
	redacted := Redact(map[string]string{"password": "secret", "runner": "worker-1"})
	if redacted["password"] != "[REDACTED]" || redacted["runner"] != "worker-1" {
		t.Fatal("redaction failed")
	}
	if !AllowedPath("/srv/tasks", "/srv/tasks/job") || AllowedPath("/srv/tasks", "/tmp") {
		t.Fatal("path boundary failed")
	}
	if !AllowedSubject("glyphflow.orders.worker-1", "worker-1") || AllowedSubject("glyphflow.orders.worker-2", "worker-1") {
		t.Fatal("subject boundary failed")
	}
}
func TestRecoveryAndTransitions(t *testing.T) {
	if !TransitionAllowed("running", "completed") || TransitionAllowed("completed", "running") {
		t.Fatal("state transition policy failed")
	}
	if RetryDelay(10, time.Second, 5*time.Second) != 5*time.Second {
		t.Fatal("retry cap failed")
	}
	if strings.TrimSpace("completed") == "" {
		t.Fatal()
	}
}
