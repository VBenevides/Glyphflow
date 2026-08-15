CREATE TABLE global_variables (
    id text PRIMARY KEY,
    name text NOT NULL,
    value text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (name ~ '^[A-Za-z_][A-Za-z0-9_]*$')
);
CREATE UNIQUE INDEX global_variables_name_ci_idx ON global_variables (lower(name));

CREATE TABLE global_variable_references (
    variable_id text NOT NULL REFERENCES global_variables(id) ON DELETE RESTRICT,
    owner_type text NOT NULL CHECK (owner_type IN ('task_version', 'schedule_version')),
    owner_id text NOT NULL,
    PRIMARY KEY (variable_id, owner_type, owner_id)
);
CREATE INDEX global_variable_references_owner_idx ON global_variable_references(owner_type, owner_id);
