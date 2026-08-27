CREATE TABLE config (
    name text PRIMARY KEY,
    value jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (name !~ '^state\.')
);

CREATE TABLE users (
    id text PRIMARY KEY,
    username text NOT NULL,
    email text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'disabled')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (email <> '' AND email = lower(btrim(email))),
    CHECK (username <> '' AND username = lower(btrim(username)))
);
CREATE UNIQUE INDEX users_username_ci_idx ON users (lower(username));
CREATE UNIQUE INDEX users_email_ci_idx ON users (lower(email));

CREATE TABLE user_passwords (
    user_id text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    password_hash text NOT NULL CHECK (password_hash ~ '^\$argon2id\$v=19\$m=[0-9]+,t=[0-9]+,p=[0-9]+\$[^$]+\$[^$]+$'),
    password_changed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash text NOT NULL UNIQUE,
    access_expires_at timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    session_family_id text NOT NULL,
    user_agent text NOT NULL DEFAULT '',
    ip_address text NOT NULL DEFAULT '',
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CHECK (refresh_expires_at >= access_expires_at)
);
CREATE INDEX auth_sessions_user_idx ON auth_sessions(user_id, refresh_expires_at);
CREATE INDEX auth_sessions_family_idx ON auth_sessions(session_family_id);

CREATE TABLE roles (
    id text PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    is_system boolean NOT NULL DEFAULT false,
    CHECK (name <> '')
);
CREATE UNIQUE INDEX roles_name_ci_idx ON roles (lower(name));

CREATE TABLE permissions (
    id text PRIMARY KEY,
    name text NOT NULL UNIQUE,
    description text NOT NULL DEFAULT ''
);
CREATE TABLE role_permissions (
    role_id text NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_id text NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);
CREATE TABLE role_assignments (
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id text NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    source_type text NOT NULL,
    source_key text NOT NULL,
    PRIMARY KEY (user_id, role_id, source_type, source_key)
);

CREATE TABLE sso_providers (
    id text PRIMARY KEY,
    name text NOT NULL,
    issuer text NOT NULL,
    client_id text NOT NULL,
    secret_reference text NOT NULL DEFAULT '',
    callback_urls jsonb NOT NULL DEFAULT '[]'::jsonb,
    auth_endpoint_override text NOT NULL DEFAULT '',
    audience text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    auto_provision boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX sso_providers_name_ci_idx ON sso_providers (lower(name));
CREATE TABLE sso_group_role_mappings (
    provider_id text NOT NULL REFERENCES sso_providers(id) ON DELETE CASCADE,
    group_name text NOT NULL,
    role_id text NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    PRIMARY KEY (provider_id, group_name, role_id)
);
CREATE TABLE user_sso_identities (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider_id text NOT NULL REFERENCES sso_providers(id) ON DELETE CASCADE,
    provider_subject text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider_id, provider_subject)
);
CREATE TABLE sso_authorization_states (
    id text PRIMARY KEY,
    provider_id text NOT NULL REFERENCES sso_providers(id) ON DELETE CASCADE,
    link_user_id text REFERENCES users(id) ON DELETE CASCADE,
    state_hash text NOT NULL UNIQUE,
    nonce_hash text NOT NULL,
    encrypted_pkce_verifier bytea NOT NULL,
    purpose text NOT NULL,
    callback_url text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sso_authorization_states_expiry_idx ON sso_authorization_states(expires_at);

CREATE TABLE audit_events (
    id text PRIMARY KEY,
    actor_id text REFERENCES users(id) ON DELETE SET NULL,
    actor_name text NOT NULL DEFAULT '',
    actor_email text NOT NULL DEFAULT '',
    method text NOT NULL,
    description text NOT NULL DEFAULT '',
    endpoint text NOT NULL,
    target text NOT NULL DEFAULT '',
    result text NOT NULL,
    request_input jsonb,
    response_output jsonb,
    before_value jsonb,
    after_value jsonb,
    traceback text,
    correlation_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_created_idx ON audit_events(created_at DESC);
CREATE INDEX audit_events_actor_idx ON audit_events(actor_id, created_at DESC);

CREATE TABLE exit_code (
    code integer PRIMARY KEY,
    meaning text NOT NULL CHECK (btrim(meaning) <> ''),
    is_system boolean NOT NULL DEFAULT false
);
INSERT INTO exit_code (code, meaning, is_system) VALUES
    (0, 'Success', true),
    (1, 'Generic/unhandled error', true),
    (2, 'Invalid arguments / usage', true),
    (5, 'Start Failure', true),
    (6, 'Timeout', true);

CREATE TABLE global_variables (
    id text PRIMARY KEY,
    name text NOT NULL CHECK (name ~ '^[A-Z_][A-Z0-9_]*$'),
    value text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX global_variables_name_ci_idx ON global_variables (lower(name));

CREATE TABLE runner_pools (
    id text PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    is_deleted boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX runner_pools_name_ci_idx ON runner_pools (lower(name)) WHERE NOT is_deleted;
CREATE INDEX runner_pools_active_idx ON runner_pools (lower(name), id) WHERE NOT is_deleted;
CREATE TABLE runners (
    id text PRIMARY KEY,
    pool_id text REFERENCES runner_pools(id) ON DELETE SET NULL,
    name text NOT NULL,
    hostname text NOT NULL DEFAULT '',
    desired_state text NOT NULL DEFAULT 'ENABLED' CHECK (desired_state IN ('ENABLED', 'DRAINING', 'DISABLED')),
    observed_state text NOT NULL DEFAULT 'PENDING' CHECK (observed_state IN ('PENDING', 'ONLINE', 'OFFLINE', 'REVOKED')),
    capacity integer NOT NULL DEFAULT 10 CHECK (capacity > 0),
    active_count integer NOT NULL DEFAULT 0 CHECK (active_count >= 0),
    capabilities jsonb NOT NULL DEFAULT '{}'::jsonb,
    nats_endpoint text NOT NULL DEFAULT '',
    control_plane_url text NOT NULL DEFAULT '',
    is_archived boolean NOT NULL DEFAULT false,
    is_deleted boolean NOT NULL DEFAULT false,
    last_seen_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX runners_name_ci_idx ON runners (lower(name)) WHERE NOT is_archived AND NOT is_deleted;
CREATE TABLE runner_sessions (
    id text PRIMARY KEY,
    runner_id text NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
    boot_id text NOT NULL,
    connected_at timestamptz NOT NULL DEFAULT now(),
    last_heartbeat_at timestamptz NOT NULL DEFAULT now(),
    current_capacity integer CHECK (current_capacity > 0),
    disconnected_at timestamptz,
    UNIQUE (runner_id, boot_id),
    UNIQUE (id, runner_id)
);
CREATE UNIQUE INDEX runner_sessions_active_idx ON runner_sessions(runner_id) WHERE disconnected_at IS NULL;
CREATE TABLE runner_metrics (
    runner_id text NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
    sampled_at timestamptz NOT NULL,
    cpu_percent double precision NOT NULL CHECK (cpu_percent >= 0 AND cpu_percent <= 100),
    memory_percent double precision NOT NULL CHECK (memory_percent >= 0 AND memory_percent <= 100),
    memory_used_bytes bigint NOT NULL CHECK (memory_used_bytes >= 0),
    memory_total_bytes bigint NOT NULL CHECK (memory_total_bytes > 0),
    PRIMARY KEY (runner_id, sampled_at)
);
CREATE INDEX runner_metrics_sampled_idx ON runner_metrics (sampled_at);
CREATE TABLE runner_keys (
    key_id text PRIMARY KEY,
    runner_id text NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
    public_key bytea NOT NULL CHECK (octet_length(public_key) > 0),
    not_before timestamptz NOT NULL,
    not_after timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (not_after IS NULL OR not_after > not_before)
);
CREATE TABLE runner_enrollments (
    id text PRIMARY KEY,
    runner_id text NOT NULL REFERENCES runners(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL CHECK (octet_length(token_hash) > 0),
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    requester text NOT NULL,
    target text NOT NULL,
    artifact jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX runner_enrollments_unused_idx ON runner_enrollments(runner_id) WHERE used_at IS NULL;

CREATE TABLE tasks (
    id text PRIMARY KEY,
    current_version_id text,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    is_deleted boolean NOT NULL DEFAULT false,
    created_by text REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, current_version_id)
);
CREATE UNIQUE INDEX tasks_name_ci_idx ON tasks (lower(name));
CREATE INDEX tasks_active_idx ON tasks (lower(name), id) WHERE NOT is_deleted;
CREATE TABLE task_versions (
    id text PRIMARY KEY,
    task_id text NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    runner_pool_id text NOT NULL REFERENCES runner_pools(id) ON DELETE RESTRICT,
    pinned_runner_id text REFERENCES runners(id) ON DELETE SET NULL,
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
    ambiguity_policy text NOT NULL DEFAULT 'REQUIRE_MANUAL_RECONCILIATION',
    execution_spec_version integer NOT NULL DEFAULT 1 CHECK (execution_spec_version > 0),
    execution_spec_digest text NOT NULL,
    created_by text REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (task_id, version),
    UNIQUE (id, task_id)
);
ALTER TABLE tasks ADD CONSTRAINT tasks_current_version_fk
    FOREIGN KEY (current_version_id, id) REFERENCES task_versions(id, task_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE schedules (
    id text PRIMARY KEY,
    task_id text NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    current_version_id text,
    name text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    next_fire_at timestamptz,
    created_by text REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, current_version_id)
);
CREATE UNIQUE INDEX schedules_name_ci_idx ON schedules (lower(name));
CREATE TABLE schedule_versions (
    id text PRIMARY KEY,
    schedule_id text NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    task_id text NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    version integer NOT NULL CHECK (version > 0),
    task_version_id text NOT NULL,
    expression text NOT NULL,
    timezone text NOT NULL,
    starts_at timestamptz,
    ends_at timestamptz,
    misfire_policy text NOT NULL,
    catchup_limit integer NOT NULL DEFAULT 0 CHECK (catchup_limit >= 0),
    start_deadline_seconds integer NOT NULL DEFAULT 0 CHECK (start_deadline_seconds >= 0),
    concurrency_policy text NOT NULL,
    max_concurrent_runs integer NOT NULL DEFAULT 0 CHECK (max_concurrent_runs >= 0),
    created_by text REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, version),
    UNIQUE (id, schedule_id),
    UNIQUE (id, task_id),
    FOREIGN KEY (task_version_id, task_id) REFERENCES task_versions(id, task_id)
);
ALTER TABLE schedules ADD CONSTRAINT schedules_current_version_fk
    FOREIGN KEY (current_version_id, id) REFERENCES schedule_versions(id, schedule_id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE runs (
    id text PRIMARY KEY,
    task_id text NOT NULL REFERENCES tasks(id) ON DELETE RESTRICT,
    task_version_id text NOT NULL,
    schedule_version_id text,
    triggered_by text REFERENCES users(id) ON DELETE SET NULL,
    trigger_type text NOT NULL CHECK (trigger_type IN ('SCHEDULE', 'MANUAL', 'RETRY')),
    scheduled_for timestamptz NOT NULL,
    resolved_global_variables jsonb NOT NULL DEFAULT '{}'::jsonb,
    start_deadline_at timestamptz,
    state text NOT NULL CHECK (state IN ('WAITING', 'DISPATCHED', 'RUNNING', 'RETRY_WAIT', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'UNKNOWN')),
    state_version bigint NOT NULL DEFAULT 0 CHECK (state_version >= 0),
    idempotency_key text NOT NULL UNIQUE,
    retry_not_before timestamptz,
    cancellation_requested_at timestamptz,
    cancellation_requested_by text REFERENCES users(id) ON DELETE SET NULL,
    cancellation_reason text,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (task_version_id, task_id) REFERENCES task_versions(id, task_id),
    FOREIGN KEY (schedule_version_id, task_id) REFERENCES schedule_versions(id, task_id)
);
CREATE UNIQUE INDEX runs_schedule_occurrence_idx ON runs(schedule_version_id, scheduled_for) WHERE schedule_version_id IS NOT NULL;

CREATE TABLE execution_attempts (
    id text PRIMARY KEY,
    run_id text NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    attempt_number integer NOT NULL CHECK (attempt_number > 0),
    runner_id text NOT NULL REFERENCES runners(id) ON DELETE RESTRICT,
    runner_session_id text NOT NULL REFERENCES runner_sessions(id) ON DELETE RESTRICT,
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
    max_memory_used_bytes bigint NOT NULL DEFAULT 0 CHECK (max_memory_used_bytes >= 0),
    average_memory_used_bytes bigint NOT NULL DEFAULT 0 CHECK (average_memory_used_bytes >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (run_id, attempt_number),
    FOREIGN KEY (runner_session_id, runner_id) REFERENCES runner_sessions(id, runner_id),
    FOREIGN KEY (exit_code) REFERENCES exit_code(code)
);

CREATE TABLE run_events (
    event_id text PRIMARY KEY,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts(id) ON DELETE CASCADE,
    state_sequence bigint NOT NULL CHECK (state_sequence > 0),
    event_kind text NOT NULL,
    reported_at timestamptz NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL,
    UNIQUE (execution_attempt_id, state_sequence)
);
CREATE TABLE execution_log_chunks (
    event_id text PRIMARY KEY,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts(id) ON DELETE CASCADE,
    stream text NOT NULL CHECK (stream IN ('stdout', 'stderr')),
    chunk_sequence bigint NOT NULL CHECK (chunk_sequence > 0),
    reported_at timestamptz NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now(),
    payload bytea NOT NULL,
    size_bytes integer NOT NULL CHECK (size_bytes >= 0),
    checksum text NOT NULL,
    UNIQUE (execution_attempt_id, stream, chunk_sequence)
);

CREATE TABLE resources (
    id text PRIMARY KEY,
    name text NOT NULL,
    kind text NOT NULL DEFAULT 'exclusive',
    next_fencing_token bigint NOT NULL DEFAULT 0 CHECK (next_fencing_token >= 0),
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX resources_name_ci_idx ON resources (lower(name));
CREATE TABLE task_resource_requirements (
    task_version_id text NOT NULL REFERENCES task_versions(id) ON DELETE CASCADE,
    resource_id text NOT NULL REFERENCES resources(id) ON DELETE RESTRICT,
    PRIMARY KEY (task_version_id, resource_id)
);
CREATE TABLE resource_leases (
    id text PRIMARY KEY,
    resource_id text NOT NULL REFERENCES resources(id) ON DELETE RESTRICT,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts(id) ON DELETE CASCADE,
    state text NOT NULL CHECK (state IN ('ACTIVE', 'RELEASED', 'EXPIRED')),
    lease_token text NOT NULL UNIQUE,
    fencing_token bigint NOT NULL CHECK (fencing_token > 0),
    acquired_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    released_at timestamptz,
    CHECK (expires_at > acquired_at)
);
CREATE UNIQUE INDEX resource_leases_active_idx ON resource_leases(resource_id) WHERE state = 'ACTIVE';

CREATE TABLE dispatch_outbox (
    message_id text PRIMARY KEY,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts(id) ON DELETE CASCADE,
    message_type text NOT NULL,
    subject text NOT NULL,
    envelope bytea NOT NULL CHECK (octet_length(envelope) > 0),
    state text NOT NULL CHECK (state IN ('PENDING', 'PUBLISHED', 'FAILED')),
    publish_attempts integer NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX dispatch_outbox_pending_idx ON dispatch_outbox(available_at, message_id) WHERE state = 'PENDING';
CREATE TABLE event_inbox (
    event_id text PRIMARY KEY,
    execution_attempt_id text NOT NULL REFERENCES execution_attempts(id) ON DELETE CASCADE,
    event_type text NOT NULL,
    subject text NOT NULL,
    envelope bytea NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE global_variable_references (
    variable_id text NOT NULL REFERENCES global_variables(id) ON DELETE RESTRICT,
    owner_type text NOT NULL CHECK (owner_type IN ('task_version', 'schedule_version')),
    owner_id text NOT NULL,
    PRIMARY KEY (variable_id, owner_type, owner_id)
);
CREATE INDEX global_variable_references_owner_idx ON global_variable_references(owner_type, owner_id);

CREATE TABLE dead_letters (
    id text PRIMARY KEY,
    runner_id text NOT NULL DEFAULT '',
    stream text NOT NULL,
    consumer text NOT NULL,
    subject text NOT NULL,
    message_id text NOT NULL,
    payload_ciphertext bytea NOT NULL CHECK (octet_length(payload_ciphertext) > 0),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    error_text text NOT NULL DEFAULT '' CHECK (octet_length(error_text) <= 4096),
    attempts integer NOT NULL CHECK (attempts > 0),
    first_failed_at timestamptz NOT NULL,
    last_failed_at timestamptz NOT NULL,
    correlation_id text NOT NULL DEFAULT '',
    state text NOT NULL DEFAULT 'OPEN' CHECK (state IN ('OPEN', 'RETRY_QUEUED', 'RECONCILED', 'DISCARDED')),
    retry_delivery_id text NOT NULL DEFAULT '',
    retry_attempts integer NOT NULL DEFAULT 0 CHECK (retry_attempts >= 0),
    retry_available_at timestamptz,
    retry_published_at timestamptz,
    retry_last_error text NOT NULL DEFAULT '' CHECK (octet_length(retry_last_error) <= 4096),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (stream, consumer, message_id)
);
CREATE INDEX dead_letters_state_idx ON dead_letters(state, last_failed_at DESC);

CREATE TABLE retention_legal_holds (
    data_class text NOT NULL CHECK (data_class IN ('run', 'dead_letter', 'audit')),
    data_id text NOT NULL,
    reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (data_class, data_id)
);

CREATE OR REPLACE FUNCTION reject_append_only_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF current_setting('glyphflow.retention_cleanup', true) = 'on' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$;
CREATE OR REPLACE FUNCTION reject_system_role_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.is_system THEN
        RAISE EXCEPTION 'system roles are immutable';
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$;
CREATE OR REPLACE FUNCTION reject_immutable_version_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION '% is immutable', TG_TABLE_NAME;
END;
$$;
CREATE OR REPLACE FUNCTION reject_schedule_version_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' AND NOT EXISTS (
        SELECT 1 FROM schedules WHERE id = OLD.schedule_id
    ) THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION '% is immutable', TG_TABLE_NAME;
END;
$$;
CREATE TRIGGER roles_system_immutable
    BEFORE UPDATE OR DELETE ON roles
    FOR EACH ROW EXECUTE FUNCTION reject_system_role_mutation();
CREATE TRIGGER task_versions_immutable
    BEFORE UPDATE OR DELETE ON task_versions
    FOR EACH ROW EXECUTE FUNCTION reject_immutable_version_mutation();
CREATE TRIGGER schedule_versions_immutable
    BEFORE UPDATE OR DELETE ON schedule_versions
    FOR EACH ROW EXECUTE FUNCTION reject_schedule_version_mutation();
CREATE TRIGGER audit_events_append_only
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER run_events_append_only
    BEFORE UPDATE OR DELETE ON run_events
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
CREATE TRIGGER execution_log_chunks_append_only
    BEFORE UPDATE OR DELETE ON execution_log_chunks
    FOR EACH ROW EXECUTE FUNCTION reject_append_only_mutation();
