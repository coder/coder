CREATE UNLOGGED TABLE workspace_app_audit_sessions (
	id UUID PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
	agent_id UUID NOT NULL,
	app_id UUID NULL,
	user_id UUID,
	ip inet,
	started_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
	FOREIGN KEY (agent_id) REFERENCES workspace_agents (id) ON DELETE CASCADE,
	FOREIGN KEY (app_id) REFERENCES workspace_apps (id) ON DELETE CASCADE
);

COMMENT ON TABLE workspace_app_audit_sessions IS 'Audit sessions for workspace apps, the data in this table is ephemeral and is used to track the current session of a user in a workspace app.';
COMMENT ON COLUMN workspace_app_audit_sessions.id IS 'Unique identifier for the workspace app audit session.';
COMMENT ON COLUMN workspace_app_audit_sessions.user_id IS 'The user that is currently using the workspace app. This is nullable because the app may be public.';
COMMENT ON COLUMN workspace_app_audit_sessions.ip IS 'The IP address of the user that is currently using the workspace app.';
COMMENT ON COLUMN workspace_app_audit_sessions.agent_id IS 'The agent that is currently in the workspace app.';
COMMENT ON COLUMN workspace_app_audit_sessions.app_id IS 'The app that is currently in the workspace app. This is nullable because ports are not associated with an app.';
COMMENT ON COLUMN workspace_app_audit_sessions.started_at IS 'The time the user started the session.';
COMMENT ON COLUMN workspace_app_audit_sessions.updated_at IS 'The time the session was last updated.';

CREATE INDEX workspace_app_audit_sessions_agent_id_app_id ON workspace_app_audit_sessions (agent_id, app_id);

COMMENT ON INDEX workspace_app_audit_sessions_agent_id_app_id IS 'Index for the agent_id and app_id columns to perform updates.';
