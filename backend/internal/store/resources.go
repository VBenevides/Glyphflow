package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ResourceRecord struct {
	ID, Name, Kind string
	Enabled        bool
	Holder         string
	ExpiresAt      *time.Time
	FencingToken   int64
}

type ResourceRepository interface {
	List(context.Context) ([]ResourceRecord, error)
	Find(context.Context, string) (ResourceRecord, bool, error)
	Create(context.Context, string, string, string) error
	Delete(context.Context, string) error
	Acquire(context.Context, string, string, time.Duration, time.Time) (ResourceRecord, error)
	Release(context.Context, string, string, int64) error
}

type ResourceStore struct{ pool *pgxpool.Pool }

func NewResourceRepository(pool *pgxpool.Pool) *ResourceStore { return &ResourceStore{pool: pool} }

const resourceQuery = `SELECT r.id, r.name, r.kind, r.enabled, COALESCE(l.execution_attempt_id, ''), r.next_fencing_token, l.expires_at FROM resources r LEFT JOIN resource_leases l ON l.resource_id = r.id AND l.state = 'ACTIVE' AND l.expires_at > now()`

func (s *ResourceStore) List(ctx context.Context) ([]ResourceRecord, error) {
	rows, err := s.pool.Query(ctx, resourceQuery+` ORDER BY lower(r.name), r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ResourceRecord{}
	for rows.Next() {
		item, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ResourceStore) Find(ctx context.Context, id string) (ResourceRecord, bool, error) {
	item, err := scanResource(s.pool.QueryRow(ctx, resourceQuery+` WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return ResourceRecord{}, false, nil
	}
	return item, err == nil, err
}

func scanResource(row interface{ Scan(...any) error }) (ResourceRecord, error) {
	var item ResourceRecord
	if err := row.Scan(&item.ID, &item.Name, &item.Kind, &item.Enabled, &item.Holder, &item.FencingToken, &item.ExpiresAt); err != nil {
		return ResourceRecord{}, err
	}
	return item, nil
}

func (s *ResourceStore) Create(ctx context.Context, id, name, kind string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO resources (id, name, kind) VALUES ($1, $2, $3)`, id, name, kind)
	return err
}

func (s *ResourceStore) Delete(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM resources r WHERE r.id = $1 AND NOT EXISTS (SELECT 1 FROM task_resource_requirements WHERE resource_id = r.id) AND NOT EXISTS (SELECT 1 FROM resource_leases WHERE resource_id = r.id AND state = 'ACTIVE' AND expires_at > now())`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("resource is missing or in use")
	}
	return nil
}

func (s *ResourceStore) Acquire(ctx context.Context, id, holder string, ttl time.Duration, now time.Time) (ResourceRecord, error) {
	if holder == "" || ttl <= 0 {
		return ResourceRecord{}, errors.New("resource lease is incomplete")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ResourceRecord{}, err
	}
	defer tx.Rollback(ctx)
	var resource ResourceRecord
	if err := tx.QueryRow(ctx, `SELECT id, name, kind, enabled, next_fencing_token FROM resources WHERE id = $1 FOR UPDATE`, id).Scan(&resource.ID, &resource.Name, &resource.Kind, &resource.Enabled, &resource.FencingToken); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ResourceRecord{}, errors.New("resource not found")
		}
		return ResourceRecord{}, err
	}
	if !resource.Enabled {
		return ResourceRecord{}, errors.New("resource is disabled")
	}
	var leaseID, leaseHolder string
	var expires time.Time
	var fencing int64
	err = tx.QueryRow(ctx, `SELECT id, execution_attempt_id, expires_at, fencing_token FROM resource_leases WHERE resource_id = $1 AND state = 'ACTIVE' ORDER BY expires_at DESC LIMIT 1 FOR UPDATE`, id).Scan(&leaseID, &leaseHolder, &expires, &fencing)
	if err == nil && now.Before(expires) {
		return ResourceRecord{}, errors.New("resource lease is active")
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ResourceRecord{}, err
	}
	if err == nil {
		if _, err := tx.Exec(ctx, `UPDATE resource_leases SET state = 'EXPIRED' WHERE id = $1`, leaseID); err != nil {
			return ResourceRecord{}, err
		}
	}
	resource.FencingToken++
	if _, err := tx.Exec(ctx, `UPDATE resources SET next_fencing_token = $2, updated_at = now() WHERE id = $1`, id, resource.FencingToken); err != nil {
		return ResourceRecord{}, err
	}
	leaseID = randomLeaseID()
	leaseToken := randomLeaseID()
	if _, err := tx.Exec(ctx, `INSERT INTO resource_leases (id, resource_id, execution_attempt_id, state, lease_token, fencing_token, acquired_at, expires_at) VALUES ($1, $2, $3, 'ACTIVE', $4, $5, $6, $7)`, leaseID, id, holder, leaseToken, resource.FencingToken, now, now.Add(ttl)); err != nil {
		return ResourceRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ResourceRecord{}, err
	}
	resource.Holder, resource.ExpiresAt = holder, timePtr(now.Add(ttl))
	return resource, nil
}

func (s *ResourceStore) Release(ctx context.Context, id, holder string, fencingToken int64) error {
	result, err := s.pool.Exec(ctx, `UPDATE resource_leases SET state = 'RELEASED' WHERE resource_id = $1 AND execution_attempt_id = $2 AND fencing_token = $3 AND state = 'ACTIVE'`, id, holder, fencingToken)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("lease owner or fencing token does not match")
	}
	return nil
}

func randomLeaseID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "lease-fallback"
	}
	return "lease-" + hex.EncodeToString(raw)
}

func timePtr(value time.Time) *time.Time { return &value }
