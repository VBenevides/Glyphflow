package platform

import (
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"time"
)

func IssueClientCertificate(ca *x509.Certificate, caKey crypto.Signer, runnerID string, publicKey crypto.PublicKey, now time.Time, validFor time.Duration) ([]byte, error) {
	if ca == nil || caKey == nil || runnerID == "" || publicKey == nil || validFor <= 0 {
		return nil, errors.New("certificate inputs are invalid")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: runnerID}, DNSNames: []string{runnerID}, NotBefore: now, NotAfter: now.Add(validFor), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, publicKey, caKey)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

func VerifyRunnerCertificate(certificatePEM []byte, ca *x509.Certificate, runnerID string, now time.Time) error {
	if len(certificatePEM) == 0 || ca == nil || runnerID == "" {
		return errors.New("certificate verification inputs are invalid")
	}
	block, _ := pem.Decode(certificatePEM)
	if block == nil {
		return errors.New("certificate is not PEM encoded")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	if err := certificate.VerifyHostname(runnerID); err != nil {
		return errors.New("certificate runner identity does not match")
	}
	if certificate.Subject.CommonName != runnerID || !certificate.NotBefore.Before(now) || !certificate.NotAfter.After(now) {
		return errors.New("certificate is not valid for runner")
	}
	if !containsExtKeyUsage(certificate.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		return errors.New("certificate is not a client certificate")
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	if _, err := certificate.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, CurrentTime: now}); err != nil {
		return err
	}
	return nil
}

func containsExtKeyUsage(usages []x509.ExtKeyUsage, wanted x509.ExtKeyUsage) bool {
	for _, usage := range usages {
		if usage == wanted {
			return true
		}
	}
	return false
}
