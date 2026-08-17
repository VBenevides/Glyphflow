ALTER TABLE runner_sessions ADD COLUMN current_capacity integer CHECK (current_capacity > 0);
