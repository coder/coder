DROP INDEX IF EXISTS idx_connection_logs_session;

ALTER TABLE connection_logs
    DROP COLUMN IF EXISTS short_description,
    DROP COLUMN IF EXISTS client_hostname,
    DROP COLUMN IF EXISTS session_id;

DROP TABLE IF EXISTS workspace_sessions;
