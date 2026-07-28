-- The 'tunnel' enum value is intentionally not removed. Postgres cannot
-- drop an enum value in place; removing it would require recreating
-- connection_type and rewriting the connection_logs type column, which
-- takes an exclusive lock on the table and would have to DELETE all
-- tunnel rows (audit data) because they cannot exist in the old type.
-- Leaving the value in place is harmless: old code never queries for it
-- and renders unknown types without error. This matches the precedent
-- of other enum-value additions (e.g. 000517, 000531).

COMMENT ON COLUMN connection_logs.user_agent IS 'Null for SSH events. For web connections, this is the User-Agent header from the request.';

COMMENT ON COLUMN connection_logs.user_id IS 'Null for SSH events. For web connections, this is the ID of the user that made the request.';

COMMENT ON COLUMN connection_logs.slug_or_port IS 'Null for SSH events. For web connections, this is the slug of the app or the port number being forwarded.';

COMMENT ON COLUMN connection_logs.disconnect_time IS 'The time the connection was closed. Null for web connections. For other connections, this is null until we receive a disconnect event for the same connection_id.';

COMMENT ON COLUMN connection_logs.disconnect_reason IS 'The reason the connection was closed. Null for web connections. For other connections, this is null until we receive a disconnect event for the same connection_id.';

COMMENT ON TABLE workspace_app_audit_sessions IS 'Audit sessions for workspace apps, the data in this table is ephemeral and is used to deduplicate audit log entries for workspace apps. While a session is active, the same data will not be logged again. This table does not store historical data.';
