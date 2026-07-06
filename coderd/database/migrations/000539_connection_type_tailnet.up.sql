ALTER TYPE connection_type ADD VALUE IF NOT EXISTS 'tailnet';

COMMENT ON COLUMN connection_logs.user_agent IS 'Null for agent-reported (SSH) events. For HTTP-initiated connections (workspace_app, port_forwarding, tailnet), this is the User-Agent header from the request.';

COMMENT ON COLUMN connection_logs.user_id IS 'Null for agent-reported (SSH) events. For HTTP-initiated connections (workspace_app, port_forwarding, tailnet), this is the ID of the user that made the request.';
