ALTER TABLE runs DROP CONSTRAINT IF EXISTS runs_state_check;
ALTER TABLE runs ADD CONSTRAINT runs_state_check CHECK (state IN ('WAITING', 'DISPATCHED', 'RUNNING', 'RETRY_WAIT', 'CANCELLING', 'SUCCEEDED', 'FAILED', 'CANCELLED', 'UNKNOWN'));

INSERT INTO exit_code (code, meaning, is_system) VALUES
    (5, 'Start Failure', true),
    (6, 'Timeout', true)
ON CONFLICT (code) DO UPDATE SET meaning = EXCLUDED.meaning, is_system = true;
