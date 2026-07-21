-- Remove the 'tunnel' value from connection_type. Postgres cannot drop an
-- enum value in place, so recreate the type without it, following the
-- pattern in 000533_nats_ca_crypto_key_feature.down.sql.
DELETE FROM connection_logs WHERE type = 'tunnel';

CREATE TYPE old_connection_type AS ENUM (
    'ssh',
    'vscode',
    'jetbrains',
    'reconnecting_pty',
    'workspace_app',
    'port_forwarding'
);

ALTER TABLE connection_logs
    ALTER COLUMN type TYPE old_connection_type
    USING (type::text::old_connection_type);

DROP TYPE connection_type;

ALTER TYPE old_connection_type RENAME TO connection_type;

COMMENT ON COLUMN connection_logs.user_agent IS 'Null for SSH events. For web connections, this is the User-Agent header from the request.';

COMMENT ON COLUMN connection_logs.user_id IS 'Null for SSH events. For web connections, this is the ID of the user that made the request.';

COMMENT ON COLUMN connection_logs.slug_or_port IS 'Null for SSH events. For web connections, this is the slug of the app or the port number being forwarded.';

COMMENT ON COLUMN connection_logs.disconnect_time IS 'The time the connection was closed. Null for web connections. For other connections, this is null until we receive a disconnect event for the same connection_id.';

COMMENT ON COLUMN connection_logs.disconnect_reason IS 'The reason the connection was closed. Null for web connections. For other connections, this is null until we receive a disconnect event for the same connection_id.';
