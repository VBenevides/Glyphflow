CREATE UNIQUE INDEX runner_enrollments_one_unused_idx
    ON runner_enrollments (runner_id)
    WHERE used_at IS NULL;
