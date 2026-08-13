CREATE INDEX task_definitions_due_idx
    ON task_definitions (next_due_at)
    WHERE enabled;

CREATE INDEX dispatch_outbox_pending_idx
    ON dispatch_outbox (available_at, id)
    WHERE state = 'pending';

CREATE INDEX task_runs_recent_idx
    ON task_runs (task_definition_id, created_at DESC);

CREATE INDEX runners_heartbeat_idx
    ON runners (heartbeat_at);

CREATE INDEX run_events_audit_idx
    ON run_events (task_run_id, accepted_at);
