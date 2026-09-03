package store

import (
	"context"
	"encoding/json"
	"time"
)

type AuditEventRecord struct {
	ID, ActorID, ActorName, ActorEmail, Method, Description, Endpoint, Target, Result string
	RequestInput, ResponseOutput, BeforeValue, AfterValue                             any
	Traceback, CorrelationID                                                          string
	CreatedAt                                                                         time.Time
}

type AuditFilter struct {
	Actor, Action, Target, Result, CorrelationID, ExcludeTarget, ExcludeResult string
	ExcludeMethod                                                              string
	ExcludeRunLogs                                                             bool
	All                                                                        bool
	From, To                                                                   time.Time
	Page, Limit                                                                int
}

type AuditCounts struct {
	Total, Failures, Writes int
}

type AuditRepository interface {
	Append(context.Context, AuditEventRecord) error
	Query(context.Context, AuditFilter) ([]AuditEventRecord, AuditCounts, error)
}

type AuditStore struct{ pool database }

const maxAuditQueryRows = 1000

func NewAuditRepository(pool any) *AuditStore {
	db, _ := databaseFrom(pool)
	return &AuditStore{pool: db}
}

func (s *AuditStore) Append(ctx context.Context, event AuditEventRecord) error {
	input, err := json.Marshal(event.RequestInput)
	if err != nil {
		return err
	}
	output, err := json.Marshal(event.ResponseOutput)
	if err != nil {
		return err
	}
	before, err := json.Marshal(event.BeforeValue)
	if err != nil {
		return err
	}
	after, err := json.Marshal(event.AfterValue)
	if err != nil {
		return err
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO audit_events (id, actor_id, actor_name, actor_email, method, description, endpoint, target, result, request_input, response_output, before_value, after_value, traceback, correlation_id, created_at) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, NULLIF($10::jsonb, 'null'::jsonb), NULLIF($11::jsonb, 'null'::jsonb), NULLIF($12::jsonb, 'null'::jsonb), NULLIF($13::jsonb, 'null'::jsonb), $14, $15, $16)`, event.ID, event.ActorID, event.ActorName, event.ActorEmail, event.Method, event.Description, event.Endpoint, event.Target, event.Result, input, output, before, after, event.Traceback, event.CorrelationID, event.CreatedAt)
	return err
}

func (s *AuditStore) Query(ctx context.Context, filter AuditFilter) ([]AuditEventRecord, AuditCounts, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.All {
		filter.All = false
		filter.Page = 1
		filter.Limit = maxAuditQueryRows
	}
	if filter.Limit <= 0 || filter.Limit > maxAuditQueryRows {
		filter.Limit = 50
	}
	where := ` WHERE ($1 = '' OR actor_id ILIKE '%' || $1 || '%' OR actor_name ILIKE '%' || $1 || '%' OR actor_email ILIKE '%' || $1 || '%') AND ($2 = '' OR method ILIKE '%' || $2 || '%') AND ($3 = '' OR target ILIKE '%' || $3 || '%') AND ($4 = '' OR result ILIKE '%' || $4 || '%') AND ($5 = '' OR correlation_id ILIKE '%' || $5 || '%') AND ($6 = '' OR upper(method) <> upper($6)) AND ($7 = '' OR target <> $7) AND ($8 = '' OR lower(result) <> lower($8)) AND ($9::timestamptz IS NULL OR created_at >= $9) AND ($10::timestamptz IS NULL OR created_at <= $10) AND ($11 = false OR (COALESCE(target, '') NOT LIKE '/api/v1/runs/%/logs%' AND COALESCE(endpoint, '') NOT LIKE '/api/v1/runs/%/logs%'))`
	var from, to any
	if !filter.From.IsZero() {
		from = filter.From
	}
	if !filter.To.IsZero() {
		to = filter.To
	}
	var counts AuditCounts
	if err := s.pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE lower(result) = 'failure'), count(*) FILTER (WHERE upper(method) IN ('POST', 'PUT', 'PATCH', 'DELETE')) FROM audit_events`+where, filter.Actor, filter.Action, filter.Target, filter.Result, filter.CorrelationID, filter.ExcludeMethod, filter.ExcludeTarget, filter.ExcludeResult, from, to, filter.ExcludeRunLogs).Scan(&counts.Total, &counts.Failures, &counts.Writes); err != nil {
		return nil, AuditCounts{}, err
	}
	query := `SELECT id, COALESCE(actor_id, ''), actor_name, actor_email, method, description, endpoint, target, result, request_input, response_output, before_value, after_value, COALESCE(traceback, ''), COALESCE(correlation_id, ''), created_at FROM audit_events` + where + ` ORDER BY created_at DESC, id DESC`
	args := []any{filter.Actor, filter.Action, filter.Target, filter.Result, filter.CorrelationID, filter.ExcludeMethod, filter.ExcludeTarget, filter.ExcludeResult, from, to, filter.ExcludeRunLogs}
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)
	query += ` LIMIT $12 OFFSET $13`
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, AuditCounts{}, err
	}
	defer rows.Close()
	events := []AuditEventRecord{}
	for rows.Next() {
		var event AuditEventRecord
		var input, output, before, after []byte
		if err := rows.Scan(&event.ID, &event.ActorID, &event.ActorName, &event.ActorEmail, &event.Method, &event.Description, &event.Endpoint, &event.Target, &event.Result, &input, &output, &before, &after, &event.Traceback, &event.CorrelationID, &event.CreatedAt); err != nil {
			return nil, AuditCounts{}, err
		}
		if err := decodeAuditValues(&event, input, output, before, after); err != nil {
			return nil, AuditCounts{}, err
		}
		events = append(events, event)
	}
	return events, counts, rows.Err()
}

func decodeAuditValues(event *AuditEventRecord, values ...[]byte) error {
	fields := []*any{&event.RequestInput, &event.ResponseOutput, &event.BeforeValue, &event.AfterValue}
	for i, raw := range values {
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		*fields[i] = value
	}
	return nil
}
