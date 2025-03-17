-- Keep all unique fields as non-null because `UNIQUE NULLS NOT DISTINCT`
-- requires PostgreSQL 15+.
CREATE UNLOGGED TABLE workspace_app_audit_sessions (
	agent_id UUID NOT NULL,
	app_id UUID NOT NULL, -- Can be NULL, but must be uuid.Nil.
	user_id UUID NOT NULL, -- Can be NULL, but must be uuid.Nil.
	ip inet NOT NULL,
	user_agent TEXT NOT NULL,
	slug_or_port TEXT NOT NULL,
	status_code int4 NOT NULL,
	started_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	FOREIGN KEY (agent_id) REFERENCES workspace_agents (id) ON DELETE CASCADE,
	-- Skip foreign keys that we can't enforce due to NOT NULL constraints.
	-- FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
	-- FOREIGN KEY (app_id) REFERENCES workspace_apps (id) ON DELETE CASCADE,
	UNIQUE (agent_id, app_id, user_id, ip, user_agent, slug_or_port, status_code)
);

COMMENT ON TABLE workspace_app_audit_sessions IS 'Audit sessions for workspace apps, the data in this table is ephemeral and is used to deduplicate audit log entries for workspace apps. While a session is active, the same data will not be logged again. This table does not store historical data.';
COMMENT ON COLUMN workspace_app_audit_sessions.agent_id IS 'The agent that the workspace app or port forward belongs to.';
COMMENT ON COLUMN workspace_app_audit_sessions.app_id IS 'The app that is currently in the workspace app. This is may be uuid.Nil because ports are not associated with an app.';
COMMENT ON COLUMN workspace_app_audit_sessions.user_id IS 'The user that is currently using the workspace app. This is may be uuid.Nil if we cannot determine the user.';
COMMENT ON COLUMN workspace_app_audit_sessions.ip IS 'The IP address of the user that is currently using the workspace app.';
COMMENT ON COLUMN workspace_app_audit_sessions.user_agent IS 'The user agent of the user that is currently using the workspace app.';
COMMENT ON COLUMN workspace_app_audit_sessions.slug_or_port IS 'The slug or port of the workspace app that the user is currently using.';
COMMENT ON COLUMN workspace_app_audit_sessions.status_code IS 'The HTTP status produced by the token authorization. Defaults to 200 if no status is provided.';
COMMENT ON COLUMN workspace_app_audit_sessions.started_at IS 'The time the user started the session.';
COMMENT ON COLUMN workspace_app_audit_sessions.updated_at IS 'The time the session was last updated.';

CREATE UNIQUE INDEX workspace_app_audit_sessions_unique_index ON workspace_app_audit_sessions (agent_id, app_id, user_id, ip, user_agent, slug_or_port, status_code);

COMMENT ON INDEX workspace_app_audit_sessions_unique_index IS 'Unique index to ensure that we do not allow duplicate entries from multiple transactions.';
