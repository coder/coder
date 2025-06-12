CREATE TYPE connection_status AS ENUM (
	'connected',
	'disconnected'
);

CREATE TYPE connection_type AS ENUM (
	-- SSH events
	'ssh',
	'vscode',
	'jetbrains',
	'reconnecting_pty',
	-- Web events
	'workspace_app',
	'port_forwarding'
);

CREATE TABLE connection_logs (
	id uuid NOT NULL,
	"time" timestamp with time zone NOT NULL,
	organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
	workspace_owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	workspace_name text NOT NULL,
	agent_name text NOT NULL,
	type connection_type NOT NULL,
	code integer,
	ip inet,

	-- Only set for web events
	user_agent text,
	user_id uuid,
	slug_or_port text,

	-- Null for web events
	connection_id uuid,
	close_time timestamp with time zone, -- Null until we upsert a disconnect log for the same connection_id.
	close_reason text,

	PRIMARY KEY (id)
);


COMMENT ON COLUMN connection_logs.code IS 'Either the HTTP status code of the web request, or the exit code of an SSH connection. For non-web connections, this is Null until we receive a disconnect event for the same connection_id.';

COMMENT ON COLUMN connection_logs.user_agent IS 'Null for SSH events. For web connections, this is the User-Agent header from the request.';

COMMENT ON COLUMN connection_logs.user_id IS 'uuid.Nil for SSH events. For web connections, this is the ID of the user that made the request.';

COMMENT ON COLUMN connection_logs.slug_or_port IS 'Null for SSH events. For web connections, this is the slug of the app or the port number being forwarded.';

COMMENT ON COLUMN connection_logs.connection_id IS 'The SSH connection ID. Used to correlate connections and disconnections. As it originates from the agent, it is not guaranteed to be unique.';

COMMENT ON COLUMN connection_logs.close_time IS 'The time the connection was closed. Null for web connections. For other connections, this is null until we receive a disconnect event for the same connection_id.';

COMMENT ON COLUMN connection_logs.close_reason IS 'The reason the connection was closed. Null for web connections. For other connections, this is null until we receive a disconnect event for the same connection_id.';

COMMENT ON TYPE audit_action IS 'NOTE: `connect`, `disconnect`, `open`, and `close` are deprecated and no longer used - these events are now tracked in the connection_logs table.';

-- To associate connection closure events with the connection start events.
CREATE UNIQUE INDEX idx_connection_logs_connection_id_workspace_id_agent_name
ON connection_logs (connection_id, workspace_id, agent_name);

CREATE INDEX idx_connection_logs_time_desc ON connection_logs USING btree ("time" DESC);
CREATE INDEX idx_connection_logs_organization_id ON connection_logs USING btree (organization_id);
CREATE INDEX idx_connection_logs_workspace_owner_id ON connection_logs USING btree (workspace_owner_id);
CREATE INDEX idx_connection_logs_workspace_id ON connection_logs USING btree (workspace_id);
