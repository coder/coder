-- Drop the trigger first
DROP TRIGGER IF EXISTS user_status_change_trigger ON users;

-- Drop the trigger function
DROP FUNCTION IF EXISTS record_user_status_change();

-- Drop the indexes
DROP INDEX IF EXISTS idx_user_status_changes_changed_at;
DROP INDEX IF EXISTS idx_user_status_changes_user_id;

-- Drop the table
DROP TABLE IF EXISTS user_status_changes;
