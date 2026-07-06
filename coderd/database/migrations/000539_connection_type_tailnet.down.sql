-- Postgres does not support removing enum values, so the down
-- migration for the `tailnet` connection_type is a no-op.

COMMENT ON COLUMN connection_logs.user_agent IS 'Null for SSH events. For web connections, this is the User-Agent header from the request.';

COMMENT ON COLUMN connection_logs.user_id IS 'Null for SSH events. For web connections, this is the ID of the user that made the request.';
