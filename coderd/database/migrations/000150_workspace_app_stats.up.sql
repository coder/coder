CREATE TABLE workspace_app_stats (
	id BIGSERIAL PRIMARY KEY,
	user_id uuid NOT NULL REFERENCES users (id),
	workspace_id uuid NOT NULL REFERENCES workspaces (id),
	agent_id uuid NOT NULL REFERENCES workspace_agents (id),
	access_method text NOT NULL,
	slug_or_port text NOT NULL,
	session_id uuid NOT NULL,
	session_started_at timestamptz NOT NULL,
	session_ended_at timestamptz NOT NULL,
	requests integer NOT NULL,

	-- Set a unique constraint to allow upserting the session_ended_at
	-- and requests fields without risk of collisions.
	UNIQUE(user_id, agent_id, session_id)
);

COMMENT ON TABLE workspace_app_stats IS 'A record of workspace app usage statistics';

COMMENT ON COLUMN workspace_app_stats.id IS 'The ID of the record';
COMMENT ON COLUMN workspace_app_stats.user_id IS 'The user who used the workspace app';
COMMENT ON COLUMN workspace_app_stats.workspace_id IS 'The workspace that the workspace app was used in';
COMMENT ON COLUMN workspace_app_stats.agent_id IS 'The workspace agent that was used';
COMMENT ON COLUMN workspace_app_stats.access_method IS 'The method used to access the workspace app';
COMMENT ON COLUMN workspace_app_stats.slug_or_port IS 'The slug or port used to to identify the app';
COMMENT ON COLUMN workspace_app_stats.session_id IS 'The unique identifier for the session';
COMMENT ON COLUMN workspace_app_stats.session_started_at IS 'The time the session started';
COMMENT ON COLUMN workspace_app_stats.session_ended_at IS 'The time the session ended';
COMMENT ON COLUMN workspace_app_stats.requests IS 'The number of requests made during the session, a number larger than 1 indicates that multiple sessions were rolled up into one';

-- Create index on workspace_id for joining/filtering by templates.
CREATE INDEX workspace_app_stats_workspace_id_idx ON workspace_app_stats (workspace_id);
