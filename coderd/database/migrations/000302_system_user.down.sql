-- Remove system user from organizations
DELETE FROM organization_members
WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';

-- Drop triggers first
DROP TRIGGER IF EXISTS prevent_system_user_updates ON users;
DROP TRIGGER IF EXISTS prevent_system_user_deletions ON users;

-- Drop function
DROP FUNCTION IF EXISTS prevent_system_user_changes();

-- Delete user status changes
DELETE FROM user_status_changes
WHERE user_id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';

-- Delete system user
DELETE FROM users
WHERE id = 'c42fdf75-3097-471c-8c33-fb52454d81c0';

-- Drop index
DROP INDEX IF EXISTS user_is_system_idx;

-- Drop column
ALTER TABLE users DROP COLUMN IF EXISTS is_system;
