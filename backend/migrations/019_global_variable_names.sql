ALTER TABLE global_variables DROP CONSTRAINT IF EXISTS global_variables_name_check;
UPDATE global_variables SET name = upper(name) WHERE name <> upper(name);
ALTER TABLE global_variables ADD CONSTRAINT global_variables_name_check CHECK (name ~ '^[A-Z_][A-Z0-9_]*$');
