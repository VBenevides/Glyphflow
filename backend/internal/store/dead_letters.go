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
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
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
	List(context.Context, DeadLetterFilter) ([]DeadLetterSummary, int, error)
	Find(context.Context, string) (DeadLetterSummary, bool, error)
	BeginRetry(context.Context, string) (DeadLetterRetry, bool, error)
	Reconcile(context.Context, string, string) (bool, error)
	Stats(context.Context) (DeadLetterStats, error)
}

type DeadLetterFilter struct {
	State, RunnerID, Subject string
	Page, Limit              int
}

type DeadLetterSummary struct {
	ID, RunnerID, Stream, Consumer, Subject, MessageID, PayloadSHA256, Error, CorrelationID, State string
	Attempts                                                                                       uint64
	FirstFailedAt, LastFailedAt                                                                    time.Time
}

type DeadLetterRetry struct {
	ID, Subject, MessageID string
	Payload                []byte
}

type DeadLetterStats struct {
	Open, OldestAgeSeconds uint64
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
	for !utf8.ValidString(record.Error) {
		record.Error = record.Error[:len(record.Error)-1]
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

const deadLetterSummarySelect = `SELECT id, runner_id, stream, consumer, subject, message_id, payload_sha256, error_text, attempts, first_failed_at, last_failed_at, correlation_id, state FROM dead_letters`

const (
	defaultDeadLetterListLimit = 50
	maxDeadLetterListLimit     = 100
)

type deadLetterScanner interface {
	Scan(...any) error
}

func scanDeadLetterSummary(scanner deadLetterScanner) (DeadLetterSummary, error) {
	var summary DeadLetterSummary
	var attempts int64
	if err := scanner.Scan(&summary.ID, &summary.RunnerID, &summary.Stream, &summary.Consumer, &summary.Subject, &summary.MessageID, &summary.PayloadSHA256, &summary.Error, &attempts, &summary.FirstFailedAt, &summary.LastFailedAt, &summary.CorrelationID, &summary.State); err != nil {
		return DeadLetterSummary{}, err
	}
	summary.Attempts = uint64(attempts)
	return summary, nil
}

func (s *DeadLetterStore) List(ctx context.Context, filter DeadLetterFilter) ([]DeadLetterSummary, int, error) {
	if s == nil || s.pool == nil {
		return nil, 0, errors.New("dead-letter storage is unavailable")
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 || filter.Limit > maxDeadLetterListLimit {
		filter.Limit = defaultDeadLetterListLimit
	}
	where := ` WHERE ($1 = '' OR state = $1) AND ($2 = '' OR runner_id = $2) AND ($3 = '' OR subject = $3)`
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM dead_letters`+where, filter.State, filter.RunnerID, filter.Subject).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Query(ctx, deadLetterSummarySelect+where+` ORDER BY last_failed_at DESC, id DESC LIMIT $4 OFFSET $5`, filter.State, filter.RunnerID, filter.Subject, filter.Limit, (filter.Page-1)*filter.Limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]DeadLetterSummary, 0, maxDeadLetterListLimit)
	for rows.Next() {
		item, err := scanDeadLetterSummary(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *DeadLetterStore) Find(ctx context.Context, id string) (DeadLetterSummary, bool, error) {
	if s == nil || s.pool == nil {
		return DeadLetterSummary{}, false, errors.New("dead-letter storage is unavailable")
	}
	item, err := scanDeadLetterSummary(s.pool.QueryRow(ctx, deadLetterSummarySelect+` WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return DeadLetterSummary{}, false, nil
	}
	return item, err == nil, err
}

func (s *DeadLetterStore) BeginRetry(ctx context.Context, id string) (DeadLetterRetry, bool, error) {
	if s == nil || s.pool == nil || id == "" {
		return DeadLetterRetry{}, false, errors.New("dead-letter storage is unavailable")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeadLetterRetry{}, false, err
	}
	defer tx.Rollback(ctx)
	var subject, messageID, state string
	var ciphertext []byte
	err = tx.QueryRow(ctx, `SELECT subject, message_id, payload_ciphertext, state FROM dead_letters WHERE id = $1 FOR UPDATE`, id).Scan(&subject, &messageID, &ciphertext, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeadLetterRetry{}, false, nil
	}
	if err != nil {
		return DeadLetterRetry{}, false, err
	}
	if state != "OPEN" {
		return DeadLetterRetry{ID: id, Subject: subject, MessageID: messageID}, false, nil
	}
	payload, err := decryptDeadLetterPayload(s.key, ciphertext)
	if err != nil {
		return DeadLetterRetry{}, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE dead_letters SET state = 'RETRY_QUEUED', updated_at = now() WHERE id = $1 AND state = 'OPEN'`, id); err != nil {
		return DeadLetterRetry{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeadLetterRetry{}, false, err
	}
	return DeadLetterRetry{ID: id, Subject: subject, MessageID: messageID, Payload: payload}, true, nil
}

func (s *DeadLetterStore) Reconcile(ctx context.Context, id, state string) (bool, error) {
	if s == nil || s.pool == nil || id == "" {
		return false, errors.New("dead-letter storage is unavailable")
	}
	if state != "RECONCILED" && state != "DISCARDED" {
		return false, errors.New("invalid dead-letter terminal state")
	}
	result, err := s.pool.Exec(ctx, `UPDATE dead_letters SET state = $1, updated_at = now() WHERE id = $2 AND state IN ('OPEN', 'RETRY_QUEUED')`, state, id)
	return result.RowsAffected() == 1, err
}

func (s *DeadLetterStore) Stats(ctx context.Context) (DeadLetterStats, error) {
	if s == nil || s.pool == nil {
		return DeadLetterStats{}, errors.New("dead-letter storage is unavailable")
	}
	var open int64
	var oldestAge float64
	if err := s.pool.QueryRow(ctx, `SELECT count(*), COALESCE(EXTRACT(EPOCH FROM now() - min(last_failed_at)), 0) FROM dead_letters WHERE state = 'OPEN'`).Scan(&open, &oldestAge); err != nil {
		return DeadLetterStats{}, err
	}
	if oldestAge < 0 {
		oldestAge = 0
	}
	return DeadLetterStats{Open: uint64(open), OldestAgeSeconds: uint64(oldestAge)}, nil
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

func decryptDeadLetterPayload(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("invalid dead-letter ciphertext")
	}
	return gcm.Open(nil, ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():], nil)
}

func newDeadLetterID() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "dead-letter-" + hex.EncodeToString(raw), nil
}
