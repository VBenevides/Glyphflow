package store

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

func TestDeadLetterRepositoryPersistsAndCASesRecovery(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("set DATABASE_URL to run PostgreSQL dead-letter repository tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := ApplyMigrations(ctx, pool, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	suffix := time.Now().UTC().Format("20060102150405.000000000")
	id := "dead-letter-test-" + suffix
	stream, consumer, messageID := "test-stream-"+suffix, "test-consumer", "test-message-"+suffix
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM dead_letters WHERE id = $1`, id) })
	repository := NewDeadLetterRepository(pool, []byte("01234567890123456789012345678901"))
	if err := repository.Persist(ctx, DeadLetterRecord{ID: id, Stream: stream, Consumer: consumer, Subject: "glyphflow.events.test", MessageID: messageID, Payload: []byte("exact-payload"), Error: "signature rejected", Attempts: 5, FirstFailedAt: time.Now().UTC(), LastFailedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	var ciphertext []byte
	if err := pool.QueryRow(ctx, `SELECT payload_ciphertext FROM dead_letters WHERE id = $1`, id).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("exact-payload")) {
		t.Fatal("dead-letter payload was stored in plaintext")
	}
	items, total, err := repository.List(ctx, DeadLetterFilter{State: "OPEN", Page: 1, Limit: 10})
	if err != nil || total < 1 || len(items) == 0 || items[0].ID != id {
		t.Fatalf("dead-letter list = %#v, total=%d, err=%v", items, total, err)
	}
	retry, claimed, err := repository.BeginRetry(ctx, id)
	if err != nil || !claimed || string(retry.Payload) != "exact-payload" {
		t.Fatalf("begin retry = %#v, claimed=%t, err=%v", retry, claimed, err)
	}
	if _, claimed, err := repository.BeginRetry(ctx, id); err != nil || claimed {
		t.Fatalf("duplicate retry claimed=%t, err=%v", claimed, err)
	}
	if changed, err := repository.Reconcile(ctx, id, "DISCARDED"); err != nil || !changed {
		t.Fatalf("reconcile changed=%t, err=%v", changed, err)
	}
}
