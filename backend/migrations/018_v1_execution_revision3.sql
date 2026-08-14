-- Revision 3 execution/control-plane schema.  The v1 suffix keeps this
-- migration additive and lets existing v0 installations migrate safely.
CREATE TABLE IF NOT EXISTS runner_pools_v1 (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS runners_v1 (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE,
    hostname text NOT NULL DEFAULT '',
    desired_state text NOT NULL DEFAULT 'ENABLED' CHECK (desired_state IN ('ENABLED','DRAINING','DISABLED')),
    observed_state text NOT NULL DEFAULT 'OFFLINE' CHECK (observed_state IN ('ONLINE','OFFLINE','REVOKED')),
    capacity integer NOT NULL DEFAULT 1 CHECK (capacity > 0),
    active_count integer NOT NULL DEFAULT 0 CHECK (active_count >= 0),
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS runner_pool_members_v1 (
    runner_pool_id text NOT NULL REFERENCES runner_pools_v1(id) ON DELETE CASCADE,
    runner_id text NOT NULL REFERENCES runners_v1(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (runner_pool_id, runner_id)
);
CREATE TABLE IF NOT EXISTS runner_sessions_v1 (
    id text PRIMARY KEY,
    runner_id text NOT NULL REFERENCES runners_v1(id) ON DELETE CASCADE,
    boot_id text NOT NULL,
    connected_at timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now(),
    disconnected_at timestamptz,
    UNIQUE (runner_id, boot_id),
    UNIQUE (id, runner_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS runner_sessions_v1_active_idx
    ON runner_sessions_v1 (runner_id) WHERE disconnected_at IS NULL;
CREATE TABLE IF NOT EXISTS runner_keys_v1 (
    key_id text PRIMARY KEY,
    runner_id text NOT NULL REFERENCES runners_v1(id) ON DELETE CASCADE,
    public_key bytea NOT NULL,
    not_before timestamptz NOT NULL,
    not_after timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (not_after IS NULL OR not_after > not_before)
);

CREATE TABLE IF NOT EXISTS tasks_v1 (
    id text PRIMARY KEY,
    current_version_id text,
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    deleted boolean NOT NULL DEFAULT false,
    created_by text REFERENCES users_v1(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, current_version_id)
);
CREATE TABLE IF NOT EXISTS task_versions_v1 (
    id text PRIMARY KEY,
    task_id text NOT NULL REFERENCES tasks_v1(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    runner_pool_id text NOT NULL REFERENCES runner_pools_v1(id),
    pinned_runner_id text REFERENCES runners_v1(id),
    placement_selectors jsonb NOT NULL DEFAULT '{}'::jsonb,
    command jsonb NOT NULL CHECK (jsonb_typeof(command) = 'array'),
    working_directory text NOT NULL DEFAULT '',
    environment jsonb NOT NULL DEFAULT '{}'::jsonb,
    secret_references jsonb NOT NULL DEFAULT '{}'::jsonb,
    timeout_seconds integer NOT NULL CHECK (timeout_seconds > 0),
    max_output_bytes bigint NOT NULL CHECK (max_output_bytes > 0),
    max_attempts integer NOT NULL DEFAULT 1 CHECK (max_attempts > 0),
    initial_backoff_seconds integer NOT NULL DEFAULT 1 CHECK (initial_backoff_seconds >= 0),
    max_backoff_seconds integer NOT NULL DEFAULT 3600 CHECK (max_backoff_seconds >= initial_backoff_seconds),
    backoff_multiplier numeric NOT NULL DEFAULT 2 CHECK (backoff_multiplier >= 1),
    retryable_exit_codes jsonb NOT NULL DEFAULT '[]'::jsonb,
    retryable_termination_reasons jsonb NOT NULL DEFAULT '[]'::jsonb,
    ambiguity_policy text NOT NULL DEFAULT 'REQUIRE_MANUAL_RECONCILIATION' CHECK (ambiguity_policy IN ('RETRY','REQUIRE_MANUAL_RECONCILIATION','MARK_FAILED')),
    execution_spec_version integer NOT NULL DEFAULT 1 CHECK (execution_spec_version > 0),
    execution_spec_digest text NOT NULL,
    created_by text REFERENCES users_v1(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (task_id, version),
    UNIQUE (id, task_id)
);
-- The composite key supports the same-parent foreign keys below.
CREATE UNIQUE INDEX IF NOT EXISTS task_versions_v1_id_task_idx ON task_versions_v1(id, task_id);
ALTER TABLE tasks_v1 DROP CONSTRAINT IF EXISTS tasks_v1_current_version_fk;
ALTER TABLE tasks_v1 ADD CONSTRAINT tasks_v1_current_version_fk
    FOREIGN KEY (current_version_id, id) REFERENCES task_versions_v1(id, task_id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE IF NOT EXISTS schedules_v1 (
    id text PRIMARY KEY,
    task_id text NOT NULL REFERENCES tasks_v1(id) ON DELETE CASCADE,
    current_version_id text,
    name text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    next_fire_at timestamptz,
    created_by text REFERENCES users_v1(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, current_version_id)
);
CREATE TABLE IF NOT EXISTS schedule_versions_v1 (
    id text PRIMARY KEY,
    schedule_id text NOT NULL REFERENCES schedules_v1(id) ON DELETE CASCADE,
    task_id text NOT NULL REFERENCES tasks_v1(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    task_version_id text NOT NULL,
    schedule_type text NOT NULL,
    expression text NOT NULL,
    timezone text NOT NULL,
    starts_at timestamptz,
    ends_at timestamptz,
    misfire_policy text NOT NULL CHECK (misfire_policy IN ('SKIP_ALL','RUN_LATEST','RUN_ALL','RUN_UP_TO_N','FAIL_AND_ALERT')),
    catchup_limit integer NOT NULL DEFAULT 0 CHECK (catchup_limit >= 0),
    start_deadline_seconds integer NOT NULL DEFAULT 0 CHECK (start_deadline_seconds >= 0),
    concurrency_policy text NOT NULL CHECK (concurrency_policy IN ('QUEUE','SKIP','REPLACE','ALLOW')),
    max_concurrent_runs integer NOT NULL DEFAULT 0 CHECK (max_concurrent_runs >= 0),
    created_by text REFERENCES users_v1(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, version),
    UNIQUE (id, schedule_id),
    UNIQUE (id, task_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES task_versions_v1(id, task_id)
);
ALTER TABLE schedules_v1 DROP CONSTRAINT IF EXISTS schedules_v1_current_version_fk;
ALTER TABLE schedules_v1 ADD CONSTRAINT schedules_v1_current_version_fk
    FOREIGN KEY (current_version_id, id) REFERENCES schedule_versions_v1(id, schedule_id) DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE IF NOT EXISTS runs_v1 (
    id text PRIMARY KEY,
    task_id text NOT NULL REFERENCES tasks_v1(id),
    task_version_id text NOT NULL,
    schedule_version_id text,
    triggered_by text REFERENCES users_v1(id),
    trigger_type text NOT NULL CHECK (trigger_type IN ('SCHEDULE','MANUAL','RETRY')),
    scheduled_for timestamptz NOT NULL,
    start_deadline_at timestamptz,
    state text NOT NULL CHECK (state IN ('WAITING','RUNNING','RETRY_WAIT','CANCELLING','SUCCEEDED','FAILED','CANCELLED','UNKNOWN')),
    state_version bigint NOT NULL DEFAULT 0 CHECK (state_version >= 0),
    idempotency_key text NOT NULL UNIQUE,
    retry_not_before timestamptz,
    cancellation_requested_at timestamptz,
    cancellation_requested_by text REFERENCES users_v1(id),
    cancellation_reason text,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (task_version_id, task_id) REFERENCES task_versions_v1(id, task_id),
    FOREIGN KEY (schedule_version_id, task_id) REFERENCES schedule_versions_v1(id, task_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS runs_v1_schedule_occurrence_idx
    ON runs_v1 (schedule_version_id, scheduled_for) WHERE schedule_version_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS execution_attempts_v1 (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES runs_v1(id) ON DELETE CASCADE,
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    runner_id text NOT NULL REFERENCES runners_v1(id),
    runner_session_id text NOT NULL REFERENCES runner_sessions_v1(id),
    state text NOT NULL,
    state_version bigint NOT NULL DEFAULT 0 CHECK (state_version >= 0),
    last_applied_state_sequence bigint NOT NULL DEFAULT 0 CHECK (last_applied_state_sequence >= 0),
    lease_token text NOT NULL UNIQUE,
    fencing_token bigint NOT NULL CHECK (fencing_token > 0),
    lease_not_after timestamptz NOT NULL,
    execution_spec_digest text NOT NULL,
    resolved_secret_versions jsonb NOT NULL DEFAULT '{}'::jsonb,
    dispatched_at timestamptz,
    accepted_at timestamptz,
    started_at timestamptz,
    last_heartbeat_at timestamptz,
    cancel_requested_at timestamptz,
    cancel_acknowledged_at timestamptz,
    finished_at timestamptz,
    exit_code integer,
    termination_reason text,
    result jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, attempt_number),
    FOREIGN KEY (runner_session_id, runner_id) REFERENCES runner_sessions_v1(id, runner_id)
);
CREATE TABLE IF NOT EXISTS resources_v1 (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE,
    kind text NOT NULL DEFAULT 'exclusive',
    next_fencing_token bigint NOT NULL DEFAULT 0 CHECK (next_fencing_token >= 0),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS task_resource_requirements_v1 (
    task_version_id text NOT NULL REFERENCES task_versions_v1(id) ON DELETE CASCADE,
    resource_id text NOT NULL REFERENCES resources_v1(id),
    PRIMARY KEY (task_version_id, resource_id)
);
CREATE TABLE IF NOT EXISTS resource_leases_v1 (
    id text PRIMARY KEY,
    resource_id text NOT NULL REFERENCES resources_v1(id),
    execution_attempt_id text NOT NULL REFERENCES execution_attempts_v1(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('ACTIVE','RELEASED','EXPIRED')),
    lease_token text NOT NULL UNIQUE,
    fencing_token bigint NOT NULL CHECK (fencing_token > 0),
    acquired_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    released_at timestamptz,
    CHECK (expires_at > acquired_at)
);
CREATE UNIQUE INDEX IF NOT EXISTS resource_leases_v1_active_idx
    ON resource_leases_v1 (resource_id) WHERE state = 'ACTIVE';

CREATE TABLE IF NOT EXISTS dispatch_outbox_v1 (
    message_id text PRIMARY KEY,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts_v1(id) ON DELETE CASCADE,
    message_type text NOT NULL,
    subject text NOT NULL,
    envelope bytea NOT NULL CHECK (octet_length(envelope) > 0),
    state text NOT NULL CHECK (state IN ('PENDING','PUBLISHED','FAILED')),
    publish_attempts integer NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS dispatch_outbox_v1_pending_idx ON dispatch_outbox_v1(available_at, message_id) WHERE state = 'PENDING';
CREATE TABLE IF NOT EXISTS event_inbox_v1 (
    event_id text PRIMARY KEY,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts_v1(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    subject text NOT NULL,
    envelope bytea NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS run_events_v1 (
    event_id text PRIMARY KEY REFERENCES event_inbox_v1(event_id) ON DELETE CASCADE,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts_v1(id) ON DELETE CASCADE,
    state_sequence bigint NOT NULL CHECK (state_sequence > 0),
    event_kind text NOT NULL,
    reported_at timestamptz NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL,
    UNIQUE (execution_attempt_id, state_sequence)
);
CREATE TABLE IF NOT EXISTS execution_log_chunks_v1 (
    event_id text PRIMARY KEY REFERENCES event_inbox_v1(event_id) ON DELETE CASCADE,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts_v1(id) ON DELETE CASCADE,
    stream text NOT NULL,
    chunk_sequence bigint NOT NULL CHECK (chunk_sequence > 0),
    reported_at timestamptz NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now(),
    payload bytea NOT NULL,
    size_bytes integer NOT NULL CHECK (size_bytes >= 0),
    checksum text NOT NULL,
    UNIQUE (execution_attempt_id, stream, chunk_sequence)
);
