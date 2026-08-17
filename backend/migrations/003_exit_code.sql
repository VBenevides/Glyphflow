CREATE TABLE exit_code (
    code integer PRIMARY KEY,
    meaning text NOT NULL CHECK (btrim(meaning) <> '')
);

INSERT INTO exit_code (code, meaning) VALUES
    (0, 'Success'),
    (1, 'Generic/unhandled error'),
    (2, 'Invalid arguments / usage')
ON CONFLICT (code) DO NOTHING;

ALTER TABLE execution_attempts
    ADD CONSTRAINT execution_attempts_exit_code_fk
    FOREIGN KEY (exit_code) REFERENCES exit_code(code);
