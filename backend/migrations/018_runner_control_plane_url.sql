ALTER TABLE runners ADD COLUMN IF NOT EXISTS control_plane_url text NOT NULL DEFAULT '';
