CREATE TABLE workload_identities (
    id uuid PRIMARY KEY,
    created_at timestamp with time zone NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    sponsor_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name text NOT NULL,
    scopes text[] NOT NULL DEFAULT '{}',
    workspace_agent_id uuid REFERENCES workspace_agents (id) ON DELETE CASCADE,
    attestation_subject text,
    bound_at timestamp with time zone,
    revoked_at timestamp with time zone
);

COMMENT ON TABLE workload_identities IS 'A workload identity is a principal for a workload that runs in a workspace. It acts with the permissions of its sponsor, narrowed to its scopes.';
COMMENT ON COLUMN workload_identities.sponsor_id IS 'The user account whose permissions bound what the workload may do.';
COMMENT ON COLUMN workload_identities.scopes IS 'API key scope names. An empty list grants nothing.';
COMMENT ON COLUMN workload_identities.workspace_agent_id IS 'The workspace daemon that runs the workload. NULL until the runtime reports where the workload runs.';
COMMENT ON COLUMN workload_identities.attestation_subject IS 'What the workload proves to act as this identity, for example a Kubernetes pod UID.';

CREATE UNIQUE INDEX workload_identities_workspace_agent_id_idx ON workload_identities (workspace_agent_id) WHERE workspace_agent_id IS NOT NULL;
CREATE INDEX workload_identities_workspace_id_idx ON workload_identities (workspace_id);
