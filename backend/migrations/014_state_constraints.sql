ALTER TABLE task_runs
    ADD CONSTRAINT task_runs_state_check
    CHECK (state IN ('queued', 'dispatched', 'accepted', 'running', 'completed', 'failed', 'timed_out', 'cancelled', 'lost'));

ALTER TABLE runners
    ADD CONSTRAINT runners_state_check
    CHECK (state IN ('enrolling', 'active', 'offline', 'disabled', 'revoked'));

ALTER TABLE run_events
    ADD CONSTRAINT run_events_type_check
    CHECK (event_type IN ('accepted', 'rejected', 'started', 'heartbeat', 'completed', 'failed', 'timed_out', 'cancelled'));

ALTER TABLE dispatch_outbox
    ADD CONSTRAINT dispatch_outbox_state_check
    CHECK (state IN ('pending', 'published', 'failed'));
