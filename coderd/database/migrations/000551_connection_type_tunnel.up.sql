ALTER TYPE connection_type ADD VALUE IF NOT EXISTS 'tunnel';

COMMENT ON COLUMN connection_logs.user_agent IS 'Null for agent-reported (SSH) events. For coderd-reported connections (workspace_app, port_forwarding, tunnel), this is the User-Agent header from the request.';

COMMENT ON COLUMN connection_logs.user_id IS 'Null for agent-reported (SSH) events. For coderd-reported connections (workspace_app, port_forwarding, tunnel), this is the ID of the user that made the request.';

COMMENT ON COLUMN connection_logs.slug_or_port IS 'Null for agent-reported (SSH) events and tunnel events. For workspace_app events, this is the slug of the app. For port_forwarding events, this is the port number being forwarded.';

COMMENT ON COLUMN connection_logs.disconnect_time IS 'The time the connection was closed. Null for coderd-reported connections (workspace_app, port_forwarding, tunnel). For agent-reported connections, this is null until we receive a disconnect event for the same connection_id.';

COMMENT ON COLUMN connection_logs.disconnect_reason IS 'The reason the connection was closed. Null for coderd-reported connections (workspace_app, port_forwarding, tunnel). For agent-reported connections, this is null until we receive a disconnect event for the same connection_id.';
