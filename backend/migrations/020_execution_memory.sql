ALTER TABLE execution_attempts ADD COLUMN IF NOT EXISTS max_memory_used_bytes bigint NOT NULL DEFAULT 0 CHECK (max_memory_used_bytes >= 0);
ALTER TABLE execution_attempts ADD COLUMN IF NOT EXISTS average_memory_used_bytes bigint NOT NULL DEFAULT 0 CHECK (average_memory_used_bytes >= 0);
