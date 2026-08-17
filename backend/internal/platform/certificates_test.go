package platform

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func TestRunnerCertificateIsBoundToRunnerIdentity(t *testing.T) {
	now := time.Now().UTC()
	caPublic, caPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ca"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPublic, caPrivate)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	runnerPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := IssueClientCertificate(ca, caPrivate, "runner-1", runnerPublic, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyRunnerCertificate(certificate, ca, "runner-1", now); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRunnerCertificate(certificate, ca, "runner-2", now); err == nil {
		t.Fatal("certificate accepted for another runner")
	}
}
