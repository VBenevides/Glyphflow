ALTER TABLE exit_code
    ADD COLUMN is_system boolean NOT NULL DEFAULT false;

UPDATE exit_code
SET is_system = true
WHERE code IN (0, 1, 2);
