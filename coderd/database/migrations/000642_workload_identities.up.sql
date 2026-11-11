CREATE TABLE workload_identities (
    id uuid PRIMARY KEY,
    created_at timestamp with time zone NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    sponsor_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    workspace_agent_id uuid NOT NULL REFERENCES workspace_agents (id) ON DELETE CASCADE,
    name text NOT NULL,
    command text NOT NULL DEFAULT '',
    scopes text[] NOT NULL DEFAULT '{}',
    runtime text NOT NULL DEFAULT '',
    sandbox_id text NOT NULL DEFAULT '',
    started_at timestamp with time zone,
    revoked_at timestamp with time zone
);

COMMENT ON TABLE workload_identities IS 'A workload identity is a principal for a workload that runs in a sandbox inside a workspace daemon. It acts with the permissions of its sponsor, narrowed to its scopes.';
COMMENT ON COLUMN workload_identities.sponsor_id IS 'The user account whose permissions bound what the workload may do.';
COMMENT ON COLUMN workload_identities.workspace_agent_id IS 'The workspace daemon that runs the workload. Only this daemon can vouch for the identity.';
COMMENT ON COLUMN workload_identities.scopes IS 'API key scope names. An empty list grants nothing.';
COMMENT ON COLUMN workload_identities.sandbox_id IS 'The sandbox the daemon reported for the workload, as its runtime names it.';
COMMENT ON COLUMN workload_identities.started_at IS 'When the daemon reported the sandbox. NULL while the workload is starting.';

CREATE INDEX workload_identities_workspace_id_idx ON workload_identities (workspace_id);
CREATE INDEX workload_identities_workspace_agent_id_idx ON workload_identities (workspace_agent_id);
