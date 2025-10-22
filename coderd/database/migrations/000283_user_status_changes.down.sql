DROP TRIGGER IF EXISTS user_status_change_trigger ON users;

DROP FUNCTION IF EXISTS record_user_status_change();

DROP INDEX IF EXISTS idx_user_status_changes_changed_at;
DROP INDEX IF EXISTS idx_user_deleted_deleted_at;

DROP TABLE IF EXISTS user_status_changes;
DROP TABLE IF EXISTS user_deleted;
