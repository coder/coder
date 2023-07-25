-- It's not possible to drop enum values from enum types, so the UP has "IF NOT EXISTS"

ALTER TABLE users ALTER COLUMN status SET DEFAULT 'active'::user_status;
