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
