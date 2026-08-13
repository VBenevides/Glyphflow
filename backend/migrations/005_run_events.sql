CREATE TABLE run_events (
    event_id text PRIMARY KEY,
    task_run_id text NOT NULL REFERENCES task_runs(id),
    attempt integer NOT NULL CHECK (attempt > 0),
    sequence bigint NOT NULL CHECK (sequence > 0),
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION reject_run_event_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'run_events is append-only';
END;
$$;

CREATE TRIGGER run_events_append_only
    BEFORE UPDATE OR DELETE ON run_events
    FOR EACH ROW EXECUTE FUNCTION reject_run_event_mutation();
