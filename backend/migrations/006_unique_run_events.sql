CREATE UNIQUE INDEX run_events_run_attempt_sequence_idx
    ON run_events (task_run_id, attempt, sequence);
