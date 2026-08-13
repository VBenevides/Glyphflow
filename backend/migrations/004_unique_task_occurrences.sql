CREATE UNIQUE INDEX task_runs_definition_occurrence_idx
    ON task_runs (task_definition_id, occurrence_at);
