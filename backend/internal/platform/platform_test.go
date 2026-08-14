package platform

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
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
	if !TransitionAllowed("running", "completed") || !TransitionAllowed("running", "unknown") || !TransitionAllowed("cancelling", "cancelled") || TransitionAllowed("completed", "running") {
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

func TestIssueClientCertificate(t *testing.T) {
	caPublic, caPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Glyphflow CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	workerPublic, _, _ := ed25519.GenerateKey(nil)
	certPEM, err := IssueClientCertificate(ca, caPrivate, "worker-1", workerPublic, now, time.Hour)
	if err != nil || len(certPEM) == 0 {
		t.Fatal(err)
	}
}

func TestAuditLogPersistsAndMetricsSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.json")
	log, err := OpenAuditLog(path)
	if err != nil {
		t.Fatal(err)
	}
	log.Add(AuditRecord{Action: "enroll"})
	reloaded, err := OpenAuditLog(path)
	if err != nil || len(reloaded.Records) != 1 {
		t.Fatalf("audit did not persist: %#v %v", reloaded.Records, err)
	}
	metrics := Metrics{}
	metrics.OrdersPublished.Add(1)
	if metrics.Snapshot()["orders_published"] != 1 {
		t.Fatal("metrics snapshot failed")
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
