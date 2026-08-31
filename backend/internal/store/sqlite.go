package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// OpenSQLite opens the control-plane database. Worker recovery uses its own
// SQLite store in internal/worker.
func OpenSQLite(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite database path is required")
	}
	if path != ":memory:" && !strings.HasPrefix(path, "file:") {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, err
		}
	}
	dsn := path
	if path == ":memory:" {
		dsn = "file::memory:?mode=memory&cache=shared"
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	db, err := sql.Open("sqlite", dsn+separator+"_pragma=busy_timeout%3d30000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=30000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func ApplySQLiteMigrations(ctx context.Context, db *sql.DB, dir string) error {
	migrations, err := LoadMigrations(dir)
	if err != nil {
		return err
	}
	if len(migrations) != 1 || migrations[0].Version != 1 || migrations[0].Name != "canonical" {
		return errors.New("exactly one canonical migration is required")
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL DEFAULT '',
		applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	migration := migrations[0]
	checksum := migrationChecksum(migration.SQL)
	var storedChecksum string
	err = db.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version = ?`, migration.Version).Scan(&storedChecksum)
	if err == nil {
		if storedChecksum != checksum {
			return fmt.Errorf("migration %d checksum changed", migration.Version)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check migration %d: %w", migration.Version, err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.Version, err)
	}
	if _, err := tx.ExecContext(ctx, sqliteSchema); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %d: %w", migration.Version, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name, checksum) VALUES (?, ?, ?)`, migration.Version, migration.Name, checksum); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %d: %w", migration.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.Version, err)
	}
	return nil
}

const sqliteSchema = `
CREATE TABLE config (name TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT NOT NULL, email TEXT NOT NULL, display_name TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'active', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX users_username_ci_idx ON users (lower(username));
CREATE UNIQUE INDEX users_email_ci_idx ON users (lower(email));
CREATE TABLE user_passwords (user_id TEXT PRIMARY KEY, password_hash TEXT NOT NULL, password_changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE auth_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, refresh_token_hash TEXT NOT NULL UNIQUE, access_expires_at TIMESTAMP NOT NULL, refresh_expires_at TIMESTAMP NOT NULL, session_family_id TEXT NOT NULL, user_agent TEXT NOT NULL DEFAULT '', ip_address TEXT NOT NULL DEFAULT '', last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, revoked_at TIMESTAMP);
CREATE INDEX auth_sessions_user_idx ON auth_sessions(user_id, refresh_expires_at);
CREATE INDEX auth_sessions_family_idx ON auth_sessions(session_family_id);
CREATE TABLE roles (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', is_system BOOLEAN NOT NULL DEFAULT FALSE);
CREATE UNIQUE INDEX roles_name_ci_idx ON roles (lower(name));
CREATE TABLE permissions (id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, description TEXT NOT NULL DEFAULT '');
CREATE TABLE role_permissions (role_id TEXT NOT NULL, permission_id TEXT NOT NULL, PRIMARY KEY (role_id, permission_id));
CREATE TABLE role_assignments (user_id TEXT NOT NULL, role_id TEXT NOT NULL, source_type TEXT NOT NULL, source_key TEXT NOT NULL, PRIMARY KEY (user_id, role_id, source_type, source_key));
CREATE TABLE sso_providers (id TEXT PRIMARY KEY, name TEXT NOT NULL, issuer TEXT NOT NULL, client_id TEXT NOT NULL, callback_urls TEXT NOT NULL DEFAULT '[]', auth_endpoint_override TEXT NOT NULL DEFAULT '', audience TEXT NOT NULL DEFAULT '', enabled BOOLEAN NOT NULL DEFAULT TRUE, auto_provision BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX sso_providers_name_ci_idx ON sso_providers (lower(name));
CREATE TABLE encrypted_secrets (id TEXT PRIMARY KEY, name TEXT NOT NULL, encrypted_value BLOB NOT NULL, integrity_status TEXT NOT NULL DEFAULT 'UNKNOWN', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, last_validated_at TIMESTAMP);
CREATE UNIQUE INDEX encrypted_secrets_name_ci_idx ON encrypted_secrets (lower(name));
CREATE TABLE sso_group_role_mappings (provider_id TEXT NOT NULL, group_name TEXT NOT NULL, role_id TEXT NOT NULL, PRIMARY KEY (provider_id, group_name, role_id));
CREATE TABLE user_sso_identities (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider_id TEXT NOT NULL, provider_subject TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (provider_id, provider_subject));
CREATE TABLE sso_authorization_states (id TEXT PRIMARY KEY, provider_id TEXT NOT NULL, link_user_id TEXT, state_hash TEXT NOT NULL UNIQUE, nonce_hash TEXT NOT NULL, encrypted_pkce_verifier BLOB NOT NULL, purpose TEXT NOT NULL, callback_url TEXT NOT NULL, expires_at TIMESTAMP NOT NULL, consumed_at TIMESTAMP, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE INDEX sso_authorization_states_expiry_idx ON sso_authorization_states(expires_at);
CREATE TABLE audit_events (id TEXT PRIMARY KEY, actor_id TEXT, actor_name TEXT NOT NULL DEFAULT '', actor_email TEXT NOT NULL DEFAULT '', method TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', endpoint TEXT NOT NULL, target TEXT NOT NULL DEFAULT '', result TEXT NOT NULL, request_input TEXT, response_output TEXT, before_value TEXT, after_value TEXT, traceback TEXT, correlation_id TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE INDEX audit_events_created_idx ON audit_events(created_at DESC);
CREATE INDEX audit_events_actor_idx ON audit_events(actor_id, created_at DESC);
CREATE TABLE exit_code (code INTEGER PRIMARY KEY, meaning TEXT NOT NULL, is_system BOOLEAN NOT NULL DEFAULT FALSE);
INSERT INTO exit_code (code, meaning, is_system) VALUES (0, 'Success', TRUE), (1, 'Generic/unhandled error', TRUE), (2, 'Invalid arguments / usage', TRUE), (5, 'Start Failure', TRUE), (6, 'Timeout', TRUE);
CREATE TABLE global_variables (id TEXT PRIMARY KEY, name TEXT NOT NULL, value TEXT NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX global_variables_name_ci_idx ON global_variables (lower(name));
CREATE TABLE runner_pools (id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', enabled BOOLEAN NOT NULL DEFAULT TRUE, is_deleted BOOLEAN NOT NULL DEFAULT FALSE, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX runner_pools_name_ci_idx ON runner_pools (lower(name)) WHERE NOT is_deleted;
CREATE INDEX runner_pools_active_idx ON runner_pools (lower(name), id) WHERE NOT is_deleted;
CREATE TABLE runners (id TEXT PRIMARY KEY, pool_id TEXT, name TEXT NOT NULL, hostname TEXT NOT NULL DEFAULT '', desired_state TEXT NOT NULL DEFAULT 'ENABLED', observed_state TEXT NOT NULL DEFAULT 'PENDING', capacity INTEGER NOT NULL DEFAULT 10, active_count INTEGER NOT NULL DEFAULT 0, capabilities TEXT NOT NULL DEFAULT '{}', nats_endpoint TEXT NOT NULL DEFAULT '', control_plane_url TEXT NOT NULL DEFAULT '', is_archived BOOLEAN NOT NULL DEFAULT FALSE, is_deleted BOOLEAN NOT NULL DEFAULT FALSE, last_seen_at TIMESTAMP, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX runners_name_ci_idx ON runners (lower(name)) WHERE NOT is_archived AND NOT is_deleted;
CREATE TABLE runner_sessions (id TEXT PRIMARY KEY, runner_id TEXT NOT NULL, boot_id TEXT NOT NULL, connected_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, last_heartbeat_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, current_capacity INTEGER, disconnected_at TIMESTAMP, UNIQUE (runner_id, boot_id), UNIQUE (id, runner_id));
CREATE UNIQUE INDEX runner_sessions_active_idx ON runner_sessions(runner_id) WHERE disconnected_at IS NULL;
CREATE TABLE runner_metrics (runner_id TEXT NOT NULL, sampled_at TIMESTAMP NOT NULL, cpu_percent REAL NOT NULL, memory_percent REAL NOT NULL, memory_used_bytes INTEGER NOT NULL, memory_total_bytes INTEGER NOT NULL, PRIMARY KEY (runner_id, sampled_at));
CREATE INDEX runner_metrics_sampled_idx ON runner_metrics (sampled_at);
CREATE TABLE runner_keys (key_id TEXT PRIMARY KEY, runner_id TEXT NOT NULL, public_key BLOB NOT NULL, not_before TIMESTAMP NOT NULL, not_after TIMESTAMP, revoked_at TIMESTAMP, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE runner_enrollments (id TEXT PRIMARY KEY, runner_id TEXT NOT NULL, token_hash BLOB NOT NULL, expires_at TIMESTAMP NOT NULL, used_at TIMESTAMP, requester TEXT NOT NULL, target TEXT NOT NULL, artifact TEXT NOT NULL DEFAULT '{}', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX runner_enrollments_unused_idx ON runner_enrollments(runner_id) WHERE used_at IS NULL;
CREATE TABLE tasks (id TEXT PRIMARY KEY, current_version_id TEXT, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '', enabled BOOLEAN NOT NULL DEFAULT TRUE, is_deleted BOOLEAN NOT NULL DEFAULT FALSE, created_by TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (id, current_version_id));
CREATE UNIQUE INDEX tasks_name_ci_idx ON tasks (lower(name));
CREATE INDEX tasks_active_idx ON tasks (lower(name), id) WHERE NOT is_deleted;
CREATE TABLE task_versions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, version INTEGER NOT NULL, runner_pool_id TEXT NOT NULL, pinned_runner_id TEXT, placement_selectors TEXT NOT NULL DEFAULT '{}', command TEXT NOT NULL, working_directory TEXT NOT NULL DEFAULT '', environment TEXT NOT NULL DEFAULT '{}', secret_references TEXT NOT NULL DEFAULT '{}', duration_seconds INTEGER NOT NULL, max_output_bytes INTEGER NOT NULL, max_attempts INTEGER NOT NULL DEFAULT 1, initial_backoff_seconds INTEGER NOT NULL DEFAULT 1, max_backoff_seconds INTEGER NOT NULL DEFAULT 3600, backoff_multiplier REAL NOT NULL DEFAULT 2, retryable_exit_codes TEXT NOT NULL DEFAULT '[]', retryable_termination_reasons TEXT NOT NULL DEFAULT '[]', ambiguity_policy TEXT NOT NULL DEFAULT 'REQUIRE_MANUAL_RECONCILIATION', execution_spec_version INTEGER NOT NULL DEFAULT 1, execution_spec_digest TEXT NOT NULL, created_by TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (task_id, version), UNIQUE (id, task_id));
CREATE TABLE schedules (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, current_version_id TEXT, name TEXT NOT NULL, enabled BOOLEAN NOT NULL DEFAULT TRUE, next_fire_at TIMESTAMP, created_by TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (id, current_version_id));
CREATE INDEX schedules_due_idx ON schedules ((next_fire_at IS NULL), next_fire_at, id) WHERE enabled;
CREATE UNIQUE INDEX schedules_name_ci_idx ON schedules (lower(name));
CREATE TABLE schedule_versions (id TEXT PRIMARY KEY, schedule_id TEXT NOT NULL, task_id TEXT NOT NULL, version INTEGER NOT NULL, task_version_id TEXT NOT NULL, expression TEXT NOT NULL, timezone TEXT NOT NULL, starts_at TIMESTAMP, ends_at TIMESTAMP, misfire_policy TEXT NOT NULL, catchup_limit INTEGER NOT NULL DEFAULT 0, start_deadline_seconds INTEGER NOT NULL DEFAULT 0, concurrency_policy TEXT NOT NULL, max_concurrent_runs INTEGER NOT NULL DEFAULT 0, created_by TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (schedule_id, version), UNIQUE (id, schedule_id), UNIQUE (id, task_id));
CREATE TABLE runs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, task_version_id TEXT NOT NULL, schedule_version_id TEXT, triggered_by TEXT, trigger_type TEXT NOT NULL, scheduled_for TIMESTAMP NOT NULL, resolved_global_variables TEXT NOT NULL DEFAULT '{}', start_deadline_at TIMESTAMP, state TEXT NOT NULL, state_version INTEGER NOT NULL DEFAULT 0, idempotency_key TEXT NOT NULL UNIQUE, retry_not_before TIMESTAMP, cancellation_requested_at TIMESTAMP, cancellation_requested_by TEXT, cancellation_reason TEXT, completed_at TIMESTAMP, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX runs_schedule_occurrence_idx ON runs(schedule_version_id, scheduled_for) WHERE schedule_version_id IS NOT NULL;
CREATE INDEX runs_dispatch_queue_idx ON runs(scheduled_for, created_at, id) WHERE state IN ('WAITING', 'RETRY_WAIT');
CREATE TABLE execution_attempts (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, attempt_number INTEGER NOT NULL, runner_id TEXT NOT NULL, runner_session_id TEXT NOT NULL, state TEXT NOT NULL, state_version INTEGER NOT NULL DEFAULT 0, last_applied_state_sequence INTEGER NOT NULL DEFAULT 0, lease_token TEXT NOT NULL UNIQUE, fencing_token INTEGER NOT NULL, lease_not_after TIMESTAMP NOT NULL, execution_spec_digest TEXT NOT NULL, resolved_secret_versions TEXT NOT NULL DEFAULT '{}', dispatched_at TIMESTAMP, accepted_at TIMESTAMP, started_at TIMESTAMP, last_heartbeat_at TIMESTAMP, cancel_requested_at TIMESTAMP, cancel_acknowledged_at TIMESTAMP, finished_at TIMESTAMP, exit_code INTEGER, termination_reason TEXT, result TEXT, max_memory_used_bytes INTEGER NOT NULL DEFAULT 0, average_memory_used_bytes INTEGER NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (run_id, attempt_number));
CREATE TABLE run_events (event_id TEXT PRIMARY KEY, execution_attempt_id TEXT NOT NULL, state_sequence INTEGER NOT NULL, event_kind TEXT NOT NULL, reported_at TIMESTAMP NOT NULL, accepted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, payload TEXT NOT NULL, UNIQUE (execution_attempt_id, state_sequence));
CREATE TABLE execution_log_chunks (event_id TEXT PRIMARY KEY, execution_attempt_id TEXT NOT NULL, stream TEXT NOT NULL, chunk_sequence INTEGER NOT NULL, reported_at TIMESTAMP NOT NULL, accepted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, payload BLOB NOT NULL, size_bytes INTEGER NOT NULL, checksum TEXT NOT NULL, UNIQUE (execution_attempt_id, stream, chunk_sequence));
CREATE TABLE resources (id TEXT PRIMARY KEY, name TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'exclusive', next_fencing_token INTEGER NOT NULL DEFAULT 0, enabled BOOLEAN NOT NULL DEFAULT TRUE, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE UNIQUE INDEX resources_name_ci_idx ON resources (lower(name));
CREATE TABLE task_resource_requirements (task_version_id TEXT NOT NULL, resource_id TEXT NOT NULL, PRIMARY KEY (task_version_id, resource_id));
CREATE TABLE resource_leases (id TEXT PRIMARY KEY, resource_id TEXT NOT NULL, execution_attempt_id TEXT NOT NULL, state TEXT NOT NULL, lease_token TEXT NOT NULL UNIQUE, fencing_token INTEGER NOT NULL, acquired_at TIMESTAMP NOT NULL, expires_at TIMESTAMP NOT NULL, released_at TIMESTAMP);
CREATE UNIQUE INDEX resource_leases_active_idx ON resource_leases(resource_id) WHERE state = 'ACTIVE';
CREATE TABLE dispatch_outbox (message_id TEXT PRIMARY KEY, execution_attempt_id TEXT NOT NULL, message_type TEXT NOT NULL, subject TEXT NOT NULL, envelope BLOB NOT NULL, state TEXT NOT NULL, publish_attempts INTEGER NOT NULL DEFAULT 0, available_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, published_at TIMESTAMP, last_error TEXT, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE INDEX dispatch_outbox_pending_idx ON dispatch_outbox(available_at, message_id) WHERE state = 'PENDING';
CREATE TABLE event_inbox (event_id TEXT PRIMARY KEY, execution_attempt_id TEXT NOT NULL, event_type TEXT NOT NULL, subject TEXT NOT NULL, envelope BLOB NOT NULL, received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE TABLE global_variable_references (variable_id TEXT NOT NULL, owner_type TEXT NOT NULL, owner_id TEXT NOT NULL, PRIMARY KEY (variable_id, owner_type, owner_id));
CREATE INDEX global_variable_references_owner_idx ON global_variable_references(owner_type, owner_id);
CREATE TABLE dead_letters (id TEXT PRIMARY KEY, runner_id TEXT NOT NULL DEFAULT '', stream TEXT NOT NULL, consumer TEXT NOT NULL, subject TEXT NOT NULL, message_id TEXT NOT NULL, payload_ciphertext BLOB NOT NULL, payload_sha256 TEXT NOT NULL, error_text TEXT NOT NULL DEFAULT '', attempts INTEGER NOT NULL, first_failed_at TIMESTAMP NOT NULL, last_failed_at TIMESTAMP NOT NULL, correlation_id TEXT NOT NULL DEFAULT '', state TEXT NOT NULL DEFAULT 'OPEN', retry_delivery_id TEXT NOT NULL DEFAULT '', retry_attempts INTEGER NOT NULL DEFAULT 0, retry_available_at TIMESTAMP, retry_published_at TIMESTAMP, retry_last_error TEXT NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE (stream, consumer, message_id));
CREATE INDEX dead_letters_state_idx ON dead_letters(state, last_failed_at DESC);
CREATE TABLE retention_legal_holds (data_class TEXT NOT NULL, data_id TEXT NOT NULL, reason TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY (data_class, data_id));
`
