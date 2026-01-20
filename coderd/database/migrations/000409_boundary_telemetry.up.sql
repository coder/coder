-- Tables to track boundary feature usage for telemetry reporting.
-- Data is collected from boundary log streams and inserted periodically by each replica.

CREATE TABLE boundary_active_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_boundary_active_users_user_id ON boundary_active_users(user_id);
CREATE INDEX idx_boundary_active_users_recorded_at ON boundary_active_users(recorded_at);

CREATE TABLE boundary_active_workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    template_id UUID NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_boundary_active_workspaces_workspace_id ON boundary_active_workspaces(workspace_id);
CREATE INDEX idx_boundary_active_workspaces_recorded_at ON boundary_active_workspaces(recorded_at);
