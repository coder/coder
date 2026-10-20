DROP INDEX IF EXISTS idx_chat_files_workspace_delivery_due;

ALTER TABLE chat_files
    DROP COLUMN IF EXISTS workspace_delivered_size_bytes,
    DROP COLUMN IF EXISTS workspace_delivered_workspace_id,
    DROP COLUMN IF EXISTS workspace_delivered_path,
    DROP COLUMN IF EXISTS workspace_delivered_at,
    DROP COLUMN IF EXISTS workspace_delivery_error,
    DROP COLUMN IF EXISTS workspace_delivery_next_attempt_at,
    DROP COLUMN IF EXISTS workspace_delivery_attempts,
    DROP COLUMN IF EXISTS purpose;

DROP TYPE IF EXISTS chat_file_purpose;
