package platform

import (
	"os"
	"path/filepath"
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
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tasks"), 0700); err != nil {
		t.Fatal(err)
	}
	if !AllowedPath(filepath.Join(root, "tasks"), filepath.Join(root, "tasks", "job")) || AllowedPath(filepath.Join(root, "tasks"), "/tmp") {
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

func TestEnrollmentClaimIsBoundAndSingleUse(t *testing.T) {
	token, hash, err := NewEnrollmentToken(32)
	if err != nil {
		t.Fatal(err)
	}
	enrollment := Enrollment{RunnerID: "worker-1", TokenHash: hash, ExpiresAt: time.Now().Add(time.Minute)}
	if err := ClaimEnrollment(&enrollment, token, "worker-1", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := ClaimEnrollment(&enrollment, token, "worker-1", time.Now()); err == nil {
		t.Fatal("enrollment token was reused")
	}
}

func TestAllowedPathRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(allowed, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(allowed, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if AllowedPath(allowed, filepath.Join(link, "file")) {
		t.Fatal("symlink escape accepted")
	}
}
