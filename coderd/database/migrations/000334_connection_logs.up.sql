CREATE TYPE connection_action AS ENUM (
	-- SSH actions
	'connect',
	'disconnect',
	-- Workspace App actions
	'open',
	'close'
);

-- Mirrors `Connection.Type` in `agent.Proto` / agentSDK.ConnectionType`
CREATE TYPE connection_type_enum AS ENUM (
	'ssh',
	'vscode',
	'jetbrains',
	'reconnecting_pty',
	'unspecified'
);

CREATE TABLE connection_logs (
	id uuid NOT NULL,
	"time" timestamp with time zone NOT NULL,
	connection_id uuid NOT NULL,
	organization_id uuid NOT NULL,
	workspace_owner_id uuid NOT NULL,
	workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE SET NULL,
	workspace_name text NOT NULL,
	agent_name text NOT NULL,
	action connection_action NOT NULL,
	code integer NOT NULL,
	ip inet,

	-- Null for SSH actions.
	user_agent text,
	user_id uuid NOT NULL, -- Can be NULL, but must be uuid.Nil.
	slug_or_port text,

	-- Null for Workspace App actions.
	connection_type connection_type_enum,
	reason text,

	PRIMARY KEY (id)
);

COMMENT ON COLUMN connection_logs.connection_id IS 'Either the workspace app request ID or the SSH connection ID. Used to correlate connections and disconnections.';

COMMENT ON COLUMN connection_logs.code IS 'Either the HTTP status code for the workspace app request, or the exit code of an SSH connection.';

COMMENT ON COLUMN connection_logs.user_agent IS 'Null for SSH actions. For workspace apps, this is the User-Agent header from the request.';

COMMENT ON COLUMN connection_logs.user_id IS 'uuid.Nil for SSH actions. For workspace apps, this is the ID of the user that made the request.';

COMMENT ON COLUMN connection_logs.connection_type IS 'Null for Workspace App actions. For SSH actions, this is the type of connection (e.g., "SSH", "VS Code").';

COMMENT ON COLUMN connection_logs.reason IS 'Null for Workspace App actions. For SSH actions, this is the reason for the connection or disconnection, to be displayed in the UI.';

COMMENT ON TYPE audit_action IS 'NOTE: `connect`, `disconnect`, `open`, and `close` are deprecated and no longer used - these events are now tracked in the connection_logs table.';

CREATE INDEX idx_connection_logs_time_desc ON connection_logs USING btree ("time" DESC);
CREATE INDEX idx_connection_logs_organization_id ON connection_logs USING btree (organization_id);
CREATE INDEX idx_connection_logs_workspace_owner_id ON connection_logs USING btree (workspace_owner_id);
CREATE INDEX idx_connection_logs_workspace_id ON connection_logs USING btree (workspace_id);
