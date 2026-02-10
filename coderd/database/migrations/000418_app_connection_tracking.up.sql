ALTER TABLE workspace_app_audit_sessions
    ADD COLUMN connection_id uuid;

ALTER TABLE connection_logs
    ADD COLUMN updated_at timestamp with time zone;

UPDATE connection_logs SET updated_at = connect_time WHERE updated_at IS NULL;

ALTER TABLE connection_logs
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT now();

COMMENT ON COLUMN connection_logs.updated_at IS
    'Last time this connection log was confirmed active. For agent connections, equals connect_time. For web connections, bumped while the session is active.';
