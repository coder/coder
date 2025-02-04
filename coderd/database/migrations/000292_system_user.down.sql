ALTER TABLE users
	DROP COLUMN IF EXISTS is_system;

DROP INDEX IF EXISTS user_is_system_idx;
