package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"modernc.org/sqlite"
)

type rowsAffecter interface {
	RowsAffected() int64
}

type scanner interface {
	Scan(...any) error
}

type databaseRows interface {
	Close() error
	Next() bool
	Scan(...any) error
	Err() error
}

type databaseTx interface {
	Exec(context.Context, string, ...any) (rowsAffecter, error)
	Query(context.Context, string, ...any) (databaseRows, error)
	QueryRow(context.Context, string, ...any) scanner
	Commit(context.Context) error
	Rollback(context.Context) error
}

type database interface {
	Exec(context.Context, string, ...any) (rowsAffecter, error)
	Query(context.Context, string, ...any) (databaseRows, error)
	QueryRow(context.Context, string, ...any) scanner
	Begin(context.Context) (databaseTx, error)
}

type postgresDatabase struct{ pool *pgxpool.Pool }

func (d postgresDatabase) Exec(ctx context.Context, query string, args ...any) (rowsAffecter, error) {
	result, err := d.pool.Exec(ctx, query, args...)
	return postgresResult{result}, err
}

func (d postgresDatabase) Query(ctx context.Context, query string, args ...any) (databaseRows, error) {
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return postgresRows{Rows: rows}, nil
}

func (d postgresDatabase) QueryRow(ctx context.Context, query string, args ...any) scanner {
	return d.pool.QueryRow(ctx, query, args...)
}

func (d postgresDatabase) Begin(ctx context.Context) (databaseTx, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return postgresTx{tx: tx}, nil
}

type postgresResult struct {
	result interface{ RowsAffected() int64 }
}

func (r postgresResult) RowsAffected() int64 { return r.result.RowsAffected() }

type postgresRows struct{ pgx.Rows }

func (r postgresRows) Close() error {
	r.Rows.Close()
	return nil
}

type postgresTx struct{ tx pgx.Tx }

func (t postgresTx) Exec(ctx context.Context, query string, args ...any) (rowsAffecter, error) {
	result, err := t.tx.Exec(ctx, query, args...)
	return postgresResult{result}, err
}

func (t postgresTx) Query(ctx context.Context, query string, args ...any) (databaseRows, error) {
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return postgresRows{Rows: rows}, nil
}

func (t postgresTx) QueryRow(ctx context.Context, query string, args ...any) scanner {
	return t.tx.QueryRow(ctx, query, args...)
}

func (t postgresTx) Commit(ctx context.Context) error   { return t.tx.Commit(ctx) }
func (t postgresTx) Rollback(ctx context.Context) error { return t.tx.Rollback(ctx) }

type sqliteResult struct{ result sql.Result }

func (r sqliteResult) RowsAffected() int64 {
	value, _ := r.result.RowsAffected()
	return value
}

func databaseFrom(value any) (database, error) {
	switch value := value.(type) {
	case database:
		return value, nil
	case *pgxpool.Pool:
		return postgresDatabase{pool: value}, nil
	case *sql.DB:
		return sqliteDatabase{db: value}, nil
	default:
		return nil, fmt.Errorf("unsupported database %T", value)
	}
}

type sqliteDatabase struct{ db *sql.DB }

func (d sqliteDatabase) Exec(ctx context.Context, query string, args ...any) (rowsAffecter, error) {
	query, args, err := sqliteQuery(query, args)
	if err != nil {
		return nil, err
	}
	result, err := d.db.ExecContext(ctx, query, args...)
	return sqliteResult{result: result}, err
}

func (d sqliteDatabase) Query(ctx context.Context, query string, args ...any) (databaseRows, error) {
	query, args, err := sqliteQuery(query, args)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqliteRows{Rows: rows}, nil
}

func (d sqliteDatabase) QueryRow(ctx context.Context, query string, args ...any) scanner {
	query, args, err := sqliteQuery(query, args)
	if err != nil {
		return sqliteRow{err: err}
	}
	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return sqliteRow{err: err}
	}
	return sqliteRow{rows: rows}
}

func (d sqliteDatabase) Begin(ctx context.Context) (databaseTx, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sqliteTx{tx: tx}, nil
}

type sqliteTx struct{ tx *sql.Tx }

func (t sqliteTx) Exec(ctx context.Context, query string, args ...any) (rowsAffecter, error) {
	query, args, err := sqliteQuery(query, args)
	if err != nil {
		return nil, err
	}
	result, err := t.tx.ExecContext(ctx, query, args...)
	return sqliteResult{result: result}, err
}

func (t sqliteTx) Query(ctx context.Context, query string, args ...any) (databaseRows, error) {
	query, args, err := sqliteQuery(query, args)
	if err != nil {
		return nil, err
	}
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return sqliteRows{Rows: rows}, nil
}

func (t sqliteTx) QueryRow(ctx context.Context, query string, args ...any) scanner {
	query, args, err := sqliteQuery(query, args)
	if err != nil {
		return sqliteRow{err: err}
	}
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return sqliteRow{err: err}
	}
	return sqliteRow{rows: rows}
}

func (t sqliteTx) Commit(context.Context) error   { return t.tx.Commit() }
func (t sqliteTx) Rollback(context.Context) error { return t.tx.Rollback() }

type sqliteRows struct{ *sql.Rows }

func (r sqliteRows) Scan(dest ...any) error {
	values := make([]any, len(dest))
	refs := make([]any, len(dest))
	for i := range values {
		refs[i] = &values[i]
	}
	if err := r.Rows.Scan(refs...); err != nil {
		return err
	}
	for i := range dest {
		if err := assignSQLite(dest[i], values[i]); err != nil {
			return err
		}
	}
	return nil
}

type sqliteRow struct {
	rows *sql.Rows
	err  error
}

func (r sqliteRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	defer r.rows.Close()
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	return sqliteRows{Rows: r.rows}.Scan(dest...)
}

func assignSQLite(destination, value any) error {
	if scanner, ok := destination.(sql.Scanner); ok {
		return scanner.Scan(value)
	}
	dst := reflect.ValueOf(destination)
	if !dst.IsValid() || dst.Kind() != reflect.Ptr || dst.IsNil() {
		return errors.New("sqlite scan destination must be a non-nil pointer")
	}
	return assignSQLiteValue(dst.Elem(), value)
}

func assignSQLiteValue(dst reflect.Value, value any) error {
	if value == nil {
		dst.Set(reflect.Zero(dst.Type()))
		return nil
	}
	if dst.Kind() == reflect.Ptr {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		return assignSQLiteValue(dst.Elem(), value)
	}
	if dst.Type() == reflect.TypeOf(time.Time{}) {
		parsed, err := sqliteTime(value)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(parsed))
		return nil
	}
	if dst.Type() == reflect.TypeOf(json.RawMessage{}) {
		raw := []byte(fmt.Sprint(value))
		if bytes, ok := value.([]byte); ok {
			raw = bytes
		}
		dst.SetBytes(raw)
		return nil
	}
	if dst.Kind() == reflect.Slice && dst.Type().Elem().Kind() == reflect.Uint8 {
		if bytes, ok := value.([]byte); ok {
			dst.SetBytes(bytes)
		} else {
			dst.SetBytes([]byte(fmt.Sprint(value)))
		}
		return nil
	}
	if dst.Kind() == reflect.Map || (dst.Kind() == reflect.Slice && dst.Type().Elem().Kind() != reflect.Uint8) || dst.Kind() == reflect.Struct {
		raw, ok := value.([]byte)
		if !ok {
			raw = []byte(fmt.Sprint(value))
		}
		return json.Unmarshal(raw, dst.Addr().Interface())
	}
	switch dst.Kind() {
	case reflect.String:
		if bytes, ok := value.([]byte); ok {
			dst.SetString(string(bytes))
		} else {
			dst.SetString(fmt.Sprint(value))
		}
	case reflect.Bool:
		switch value := value.(type) {
		case bool:
			dst.SetBool(value)
		case int64:
			dst.SetBool(value != 0)
		default:
			parsed, err := strconv.ParseBool(fmt.Sprint(value))
			if err != nil {
				return err
			}
			dst.SetBool(parsed)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(fmt.Sprint(value), 10, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetInt(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(fmt.Sprint(value), dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetFloat(parsed)
	default:
		return fmt.Errorf("unsupported sqlite scan destination %s", dst.Type())
	}
	return nil
}

func sqliteTime(value any) (time.Time, error) {
	if parsed, ok := value.(time.Time); ok {
		return parsed, nil
	}
	text := fmt.Sprint(value)
	if text == "epoch" {
		return time.Unix(0, 0).UTC(), nil
	}
	legacyText := text
	if fields := strings.Fields(text); len(fields) == 4 {
		legacyText = strings.Join(fields[:3], " ")
	}
	for _, candidate := range []struct {
		text, layout string
	}{
		{text, time.RFC3339Nano},
		{text, time.RFC3339},
		{text, "2006-01-02 15:04:05.999999999-07:00"},
		{text, "2006-01-02 15:04:05"},
		{legacyText, "2006-01-02 15:04:05.999999999 -0700"},
		{legacyText, "2006-01-02 15:04:05 -0700"},
	} {
		if parsed, err := time.Parse(candidate.layout, candidate.text); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid sqlite timestamp %q", text)
}

var sqliteCast = regexp.MustCompile(`::[A-Za-z_][A-Za-z0-9_]*(?:\[\])?`)
var sqlitePlaceholder = regexp.MustCompile(`\$(\d+)(?:_(\d+))?`)
var sqliteAny = regexp.MustCompile(`(?i)([A-Za-z0-9_.]+)\s*=\s*ANY\(\$(\d+)\)`)
var sqliteDecode = regexp.MustCompile(`(?i)decode\(\$(\d+),\s*'hex'\)`)

func sqliteQuery(query string, args []any) (string, []any, error) {
	args = append([]any(nil), args...)
	query = sqliteCast.ReplaceAllString(query, "")
	query = strings.ReplaceAll(query, "ILIKE", "LIKE")
	query = strings.ReplaceAll(query, "ilike", "like")
	query = strings.ReplaceAll(query, "LATERAL ", "")
	query = strings.ReplaceAll(query, "FOR UPDATE OF r, rr, rs", "")
	query = strings.ReplaceAll(query, "FOR UPDATE OF r, a", "")
	query = strings.ReplaceAll(query, "FOR UPDATE OF s", "")
	query = strings.ReplaceAll(query, "FOR UPDATE", "")
	query = strings.ReplaceAll(query, "FOR SHARE", "")
	query = strings.ReplaceAll(query, "SKIP LOCKED", "")
	query = strings.ReplaceAll(query, "jsonb_", "json_")
	query = strings.ReplaceAll(query, "json_agg(", "json_group_array(")
	query = strings.ReplaceAll(query, "json_group_array(req.resource_id ORDER BY req.resource_id)", "json_group_array(req.resource_id)")
	query = strings.ReplaceAll(query, "json_group_array(json_object('id', usage.id, 'name', usage.name) ORDER BY lower(usage.name), usage.id)", "json_group_array(json_object('id', usage.id, 'name', usage.name))")
	query = strings.ReplaceAll(query, "json_build_object(", "json_object(")
	query = strings.ReplaceAll(query, "json_each_text(", "json_each(")
	query = strings.ReplaceAll(query, "current_setting('glyphflow.retention_cleanup', true)", "'on'")
	query = strings.ReplaceAll(query, "set_config('glyphflow.retention_cleanup', 'on', true)", "'on'")
	query = strings.ReplaceAll(query, "now() + ($2 * interval '1 second')", "datetime(CURRENT_TIMESTAMP, '+' || $2 || ' seconds')")
	query = strings.ReplaceAll(query, "now() + $3 * interval '1 second'", "datetime(CURRENT_TIMESTAMP, '+' || $3 || ' seconds')")
	query = strings.ReplaceAll(query, "a.dispatched_at + $1 * interval '1 second'", "datetime(a.dispatched_at, '+' || $1 || ' seconds')")
	query = strings.ReplaceAll(query, "a.dispatched_at + $3 * interval '1 second'", "datetime(a.dispatched_at, '+' || $3 || ' seconds')")
	query = strings.ReplaceAll(query, "a.dispatched_at + (tv.duration_seconds * interval '1 second') + interval '10 minutes'", "datetime(a.dispatched_at, '+' || tv.duration_seconds || ' seconds', '+10 minutes')")
	query = strings.ReplaceAll(query, "now() - interval '30 seconds'", "datetime(CURRENT_TIMESTAMP, '-30 seconds')")
	query = strings.ReplaceAll(query, "now() + interval '1 second'", "datetime(CURRENT_TIMESTAMP, '+1 second')")
	query = strings.ReplaceAll(query, "now()", "CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "r.scheduled_for <= CURRENT_TIMESTAMP", "datetime(r.scheduled_for) <= CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "r.retry_not_before <= CURRENT_TIMESTAMP", "datetime(r.retry_not_before) <= CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "lease.expires_at > CURRENT_TIMESTAMP", "datetime(lease.expires_at) > CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "l.expires_at > CURRENT_TIMESTAMP", "datetime(l.expires_at) > CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "access_expires_at > CURRENT_TIMESTAMP", "datetime(access_expires_at) > CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "refresh_expires_at > CURRENT_TIMESTAMP", "datetime(refresh_expires_at) > CURRENT_TIMESTAMP")
	query = strings.ReplaceAll(query, "expires_at > CURRENT_TIMESTAMP", "datetime(expires_at) > CURRENT_TIMESTAMP")
	for _, match := range sqliteDecode.FindAllStringSubmatch(query, -1) {
		index, err := strconv.Atoi(match[1])
		if err != nil || index < 1 || index > len(args) {
			return "", nil, fmt.Errorf("invalid sqlite decode placeholder $%s", match[1])
		}
		hexValue, ok := args[index-1].(string)
		if !ok {
			return "", nil, errors.New("sqlite decode argument must be hexadecimal text")
		}
		decoded, err := hex.DecodeString(hexValue)
		if err != nil {
			return "", nil, err
		}
		args[index-1] = decoded
	}
	query = sqliteDecode.ReplaceAllString(query, "$$${1}")
	query = strings.ReplaceAll(query, "convert_from(", "CAST(")
	query = strings.ReplaceAll(query, ", 'UTF8')", " AS TEXT)")
	query = strings.ReplaceAll(query, "GREATEST(", "MAX(")
	query = strings.ReplaceAll(query, "rr.capabilities @> task.selectors", "json_contains(rr.capabilities, task.selectors)")
	query = strings.ReplaceAll(query, "rr.capabilities @> COALESCE(tv.placement_selectors, '{}')", "json_contains(rr.capabilities, COALESCE(tv.placement_selectors, '{}'))")
	query = strings.ReplaceAll(query, "interval '1 second'", "' seconds'")
	query = strings.ReplaceAll(query, "interval '30 seconds'", "' seconds'")
	query = strings.ReplaceAll(query, "interval '1 hour'", "' hours'")
	query = strings.ReplaceAll(query, "interval '1 day'", "' days'")
	query = strings.ReplaceAll(query, "interval '1 month'", "' months'")
	query = strings.ReplaceAll(query, "interval '1 year'", "' years'")
	query = strings.ReplaceAll(query, "EXTRACT(EPOCH FROM CURRENT_TIMESTAMP - min(last_failed_at))", "(julianday(CURRENT_TIMESTAMP) - julianday(min(last_failed_at))) * 86400")
	query = strings.ReplaceAll(query, "DELETE FROM runner_metrics m USING candidates c WHERE m.runner_id = c.runner_id AND m.sampled_at = c.sampled_at", "DELETE FROM runner_metrics WHERE (runner_id, sampled_at) IN (SELECT runner_id, sampled_at FROM candidates)")
	query = regexp.MustCompile(`(?is)DELETE FROM (\w+) \w+ USING candidates \w+ WHERE \w+\.id = \w+\.id`).ReplaceAllString(query, `DELETE FROM $1 WHERE id IN (SELECT id FROM candidates)`)
	query = sqliteAny.ReplaceAllStringFunc(query, func(match string) string {
		parts := sqliteAny.FindStringSubmatch(match)
		items := sqliteSlice(args, parts[2])
		placeholders := make([]string, len(items))
		for i := range placeholders {
			placeholders[i] = "$" + parts[2] + "_" + strconv.Itoa(i)
		}
		return parts[1] + " IN (" + strings.Join(placeholders, ",") + ")"
	})
	query = sqliteDecode.ReplaceAllString(query, "$$${1}")

	var output strings.Builder
	outputArgs := make([]any, 0, len(args))
	last := 0
	for _, match := range sqlitePlaceholder.FindAllStringSubmatchIndex(query, -1) {
		output.WriteString(query[last:match[0]])
		index, err := strconv.Atoi(query[match[2]:match[3]])
		if err != nil || index < 1 || index > len(args) {
			return "", nil, fmt.Errorf("invalid sqlite placeholder $%s", query[match[2]:match[3]])
		}
		value := args[index-1]
		if match[4] >= 0 {
			item, err := strconv.Atoi(query[match[4]:match[5]])
			if err != nil {
				return "", nil, err
			}
			items := sqliteSlice(args, strconv.Itoa(index))
			if item < 0 || item >= len(items) {
				return "", nil, errors.New("sqlite ANY item is out of range")
			}
			value = items[item]
		}
		switch timestamp := value.(type) {
		case time.Time:
			value = timestamp.UTC().Format("2006-01-02 15:04:05.999999999")
		case *time.Time:
			if timestamp != nil {
				value = timestamp.UTC().Format("2006-01-02 15:04:05.999999999")
			}
		}
		output.WriteByte('?')
		outputArgs = append(outputArgs, value)
		last = match[1]
	}
	output.WriteString(query[last:])
	return output.String(), outputArgs, nil
}

func init() {
	_ = sqliteRegisterJSONContains()
}

func sqliteRegisterJSONContains() error {
	return sqlite.RegisterDeterministicScalarFunction("json_contains", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		var container, subset map[string]any
		if len(args) != 2 || json.Unmarshal(sqliteJSON(args[0]), &container) != nil || json.Unmarshal(sqliteJSON(args[1]), &subset) != nil {
			return int64(0), nil
		}
		for key, value := range subset {
			actual, ok := container[key]
			if !ok || !reflect.DeepEqual(actual, value) {
				return int64(0), nil
			}
		}
		return int64(1), nil
	})
}

func sqliteJSON(value driver.Value) []byte {
	switch value := value.(type) {
	case []byte:
		return value
	case string:
		return []byte(value)
	default:
		return []byte(fmt.Sprint(value))
	}
}

func sqliteSlice(args []any, placeholder string) []any {
	index, _ := strconv.Atoi(placeholder)
	if index < 1 || index > len(args) {
		return nil
	}
	value := reflect.ValueOf(args[index-1])
	if !value.IsValid() || (value.Kind() != reflect.Slice && value.Kind() != reflect.Array) {
		return nil
	}
	items := make([]any, value.Len())
	for i := range items {
		items[i] = value.Index(i).Interface()
	}
	return items
}
