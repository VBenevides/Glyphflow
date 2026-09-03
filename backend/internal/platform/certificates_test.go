package platform

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
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
	for _, test := range []struct {
		name string
		ca   *x509.Certificate
		key  crypto.Signer
		id   string
		pub  crypto.PublicKey
		when time.Time
	}{
		{name: "missing CA", key: caPrivate, id: "runner-1", pub: runnerPublic, when: now, ca: nil},
		{name: "missing CA key", ca: ca, id: "runner-1", pub: runnerPublic, when: now},
		{name: "missing runner", ca: ca, key: caPrivate, pub: runnerPublic, when: now},
		{name: "missing public key", ca: ca, key: caPrivate, id: "runner-1", when: now},
		{name: "invalid duration", ca: ca, key: caPrivate, id: "runner-1", pub: runnerPublic, when: now},
	} {
		t.Run("issue-"+test.name, func(t *testing.T) {
			validFor := time.Hour
			if test.name == "invalid duration" {
				validFor = 0
			}
			if _, err := IssueClientCertificate(test.ca, test.key, test.id, test.pub, test.when, validFor); err == nil {
				t.Fatal("invalid certificate inputs were accepted")
			}
		})
	}
	for _, test := range []struct {
		name string
		cert []byte
		ca   *x509.Certificate
		id   string
		when time.Time
	}{
		{name: "missing certificate", ca: ca, id: "runner-1", when: now},
		{name: "not PEM", cert: []byte("invalid"), ca: ca, id: "runner-1", when: now},
		{name: "expired", cert: certificate, ca: ca, id: "runner-1", when: now.Add(2 * time.Hour)},
		{name: "missing CA", cert: certificate, id: "runner-1", when: now},
	} {
		t.Run("verify-"+test.name, func(t *testing.T) {
			if err := VerifyRunnerCertificate(test.cert, test.ca, test.id, test.when); err == nil {
				t.Fatal("invalid certificate was accepted")
			}
		})
	}
	serverTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "runner-1"}, DNSNames: []string{"runner-1"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, ca, runnerPublic, caPrivate)
	if err != nil {
		t.Fatal(err)
	}
	serverCertificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	if err := VerifyRunnerCertificate(serverCertificate, ca, "runner-1", now); err == nil {
		t.Fatal("server certificate accepted as runner certificate")
	}
}
