package store

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"testing"
)

func TestDeadLetterPayloadIsEncryptedAndBounded(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	payload := []byte("sensitive order payload")
	ciphertext, err := encryptDeadLetterPayload(key, payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, payload) || bytes.Equal(ciphertext, payload) {
		t.Fatal("dead-letter payload was stored in plaintext")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
	if err != nil || !bytes.Equal(plain, payload) {
		t.Fatalf("encrypted payload could not be recovered: %v", err)
	}
}

func TestDeadLetterPersistenceRejectsIncompleteRecord(t *testing.T) {
	if err := (&DeadLetterStore{}).Persist(nil, DeadLetterRecord{}); err == nil {
		t.Fatal("incomplete dead-letter record was accepted")
	}
}
