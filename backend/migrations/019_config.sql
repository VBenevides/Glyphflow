CREATE TABLE IF NOT EXISTS config (
    name text PRIMARY KEY,
    value jsonb NOT NULL
);
