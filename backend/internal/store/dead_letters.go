package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DeadLetterRecord struct {
	ID, RunnerID, Stream, Consumer, Subject, MessageID, Error, CorrelationID, State string
	Payload                                                                         []byte
	Attempts                                                                        uint64
	FirstFailedAt, LastFailedAt                                                     time.Time
}

type DeadLetterRepository interface {
	Persist(context.Context, DeadLetterRecord) error
}

type DeadLetterStore struct {
	pool *pgxpool.Pool
	key  []byte
}

func NewDeadLetterRepository(pool *pgxpool.Pool, applicationSecret []byte) *DeadLetterStore {
	digest := sha256.Sum256(append([]byte("glyphflow dead-letter payload key\x00"), applicationSecret...))
	return &DeadLetterStore{pool: pool, key: digest[:]}
}

func (s *DeadLetterStore) Persist(ctx context.Context, record DeadLetterRecord) error {
	if s == nil || s.pool == nil || len(record.Payload) == 0 || record.Stream == "" || record.Consumer == "" || record.Subject == "" || record.MessageID == "" || record.Attempts == 0 {
		return errors.New("dead-letter record is incomplete")
	}
	if record.State == "" {
		record.State = "OPEN"
	}
	if record.FirstFailedAt.IsZero() {
		record.FirstFailedAt = time.Now().UTC()
	}
	if record.LastFailedAt.IsZero() {
		record.LastFailedAt = record.FirstFailedAt
	}
	if len(record.Error) > 4096 {
		record.Error = record.Error[:4096]
	}
	ciphertext, err := encryptDeadLetterPayload(s.key, record.Payload)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(record.Payload)
	if record.ID == "" {
		record.ID, err = newDeadLetterID()
		if err != nil {
			return err
		}
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO dead_letters (id, runner_id, stream, consumer, subject, message_id, payload_ciphertext, payload_sha256, error_text, attempts, first_failed_at, last_failed_at, correlation_id, state) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) ON CONFLICT (stream, consumer, message_id) DO NOTHING`, record.ID, record.RunnerID, record.Stream, record.Consumer, record.Subject, record.MessageID, ciphertext, hex.EncodeToString(digest[:]), record.Error, record.Attempts, record.FirstFailedAt, record.LastFailedAt, record.CorrelationID, record.State)
	return err
}

func encryptDeadLetterPayload(key, payload []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, payload, nil), nil
}

func newDeadLetterID() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "dead-letter-" + hex.EncodeToString(raw), nil
}
