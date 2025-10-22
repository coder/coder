CREATE TYPE workspace_app_status_state AS ENUM ('working', 'complete', 'failure');

-- Workspace app statuses allow agents to report statuses per-app in the UI.
CREATE TABLE workspace_app_statuses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- The agent that the status is for.
    agent_id UUID NOT NULL REFERENCES workspace_agents(id),
    -- The slug of the app that the status is for. This will be used
    -- to reference the app in the UI - with an icon.
    app_id UUID NOT NULL REFERENCES workspace_apps(id),
	-- workspace_id is the workspace that the status is for.
	workspace_id UUID NOT NULL REFERENCES workspaces(id),
    -- The status determines how the status is displayed in the UI.
    state workspace_app_status_state NOT NULL,
    -- Whether the status needs user attention.
    needs_user_attention BOOLEAN NOT NULL,
    -- The message is the main text that will be displayed in the UI.
    message TEXT NOT NULL,
    -- The URI of the resource that the status is for.
    -- e.g. https://github.com/org/repo/pull/123
    -- e.g. file:///path/to/file
    uri TEXT,
    -- Icon is an external URL to an icon that will be rendered in the UI.
    icon TEXT
);

CREATE INDEX idx_workspace_app_statuses_workspace_id_created_at ON workspace_app_statuses(workspace_id, created_at DESC);
