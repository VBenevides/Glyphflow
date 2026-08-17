package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GlobalVariableRecord struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Value      string    `json:"value"`
	UpdatedAt  time.Time `json:"updatedAt"`
	References int       `json:"references"`
}

type GlobalVariableRepository interface {
	List(context.Context) ([]GlobalVariableRecord, error)
	Find(context.Context, string) (GlobalVariableRecord, bool, error)
	Create(context.Context, string, string, string) (GlobalVariableRecord, error)
	Update(context.Context, string, string, string) (GlobalVariableRecord, error)
	Delete(context.Context, string) error
}

type GlobalVariableStore struct{ pool *pgxpool.Pool }

func NewGlobalVariableRepository(pool *pgxpool.Pool) *GlobalVariableStore {
	return &GlobalVariableStore{pool: pool}
}

const globalVariableQuery = `SELECT g.id, g.name, g.value, g.updated_at, (SELECT count(*) FROM global_variable_references r WHERE r.variable_id = g.id) FROM global_variables g`

func (s *GlobalVariableStore) List(ctx context.Context) ([]GlobalVariableRecord, error) {
	rows, err := s.pool.Query(ctx, globalVariableQuery+` ORDER BY lower(g.name), g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []GlobalVariableRecord{}
	for rows.Next() {
		item, err := scanGlobalVariable(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *GlobalVariableStore) Find(ctx context.Context, id string) (GlobalVariableRecord, bool, error) {
	item, err := scanGlobalVariable(s.pool.QueryRow(ctx, globalVariableQuery+` WHERE g.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return GlobalVariableRecord{}, false, nil
	}
	return item, err == nil, err
}

func scanGlobalVariable(row interface{ Scan(...any) error }) (GlobalVariableRecord, error) {
	var item GlobalVariableRecord
	var references int64
	if err := row.Scan(&item.ID, &item.Name, &item.Value, &item.UpdatedAt, &references); err != nil {
		return GlobalVariableRecord{}, err
	}
	item.References = int(references)
	return item, nil
}

func (s *GlobalVariableStore) Create(ctx context.Context, id, name, value string) (GlobalVariableRecord, error) {
	if _, err := s.pool.Exec(ctx, `INSERT INTO global_variables (id, name, value) VALUES ($1, $2, $3)`, id, strings.TrimSpace(name), value); err != nil {
		return GlobalVariableRecord{}, err
	}
	item, _, err := s.Find(ctx, id)
	return item, err
}

func (s *GlobalVariableStore) Update(ctx context.Context, id, name, value string) (GlobalVariableRecord, error) {
	result, err := s.pool.Exec(ctx, `UPDATE global_variables SET name = $2, value = $3, updated_at = now() WHERE id = $1 AND (name = $2 OR NOT EXISTS (SELECT 1 FROM global_variable_references WHERE variable_id = $1))`, id, strings.TrimSpace(name), value)
	if err != nil {
		return GlobalVariableRecord{}, err
	}
	if result.RowsAffected() == 0 {
		return GlobalVariableRecord{}, errors.New("global variable is missing or its name is referenced")
	}
	item, _, err := s.Find(ctx, id)
	return item, err
}

func (s *GlobalVariableStore) Delete(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM global_variables WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("global variable not found")
	}
	return nil
}
