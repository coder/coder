CREATE TABLE workload_identities (
    id uuid PRIMARY KEY,
    created_at timestamp with time zone NOT NULL,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    sponsor_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    workspace_agent_id uuid REFERENCES workspace_agents (id) ON DELETE CASCADE,
    user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    name text NOT NULL,
    command text NOT NULL DEFAULT '',
    scopes text[] NOT NULL DEFAULT '{}',
    runtime text NOT NULL DEFAULT '',
    sandbox_id text NOT NULL DEFAULT '',
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    end_reason text NOT NULL DEFAULT '',
    exit_code integer,
    output_tail text NOT NULL DEFAULT '',
    interactive boolean NOT NULL DEFAULT false,
    remove_workspace boolean NOT NULL DEFAULT false
);

COMMENT ON TABLE workload_identities IS 'A workload identity is a principal for a workload. It is sponsored by a user account and can do nothing beyond its scopes. The row outlives the workload as the record of what ran.';
COMMENT ON COLUMN workload_identities.sponsor_id IS 'The user account that asked for the workload and answers for it.';
COMMENT ON COLUMN workload_identities.workspace_agent_id IS 'The workspace daemon that runs the workload. Only this daemon can vouch for the identity. NULL while the workspace that was made for the workload is being built.';
COMMENT ON COLUMN workload_identities.user_id IS 'The service account that is this identity. It owns the workspace that was made for the workload, and the workload acts as it, narrowed to its scopes. NULL once the account is gone.';
COMMENT ON COLUMN workload_identities.scopes IS 'API key scope names. An empty list grants nothing.';
COMMENT ON COLUMN workload_identities.sandbox_id IS 'Where the daemon reported that the workload runs, as its runtime names it.';
COMMENT ON COLUMN workload_identities.started_at IS 'When the daemon reported that the workload runs. NULL while the workload waits for its daemon to start it.';
COMMENT ON COLUMN workload_identities.finished_at IS 'When the workload was over. From then on the identity authorizes nothing. NULL while the workload is live.';
COMMENT ON COLUMN workload_identities.end_reason IS 'Why the workload is over: exited, removed, stopped, lost or failed.';
COMMENT ON COLUMN workload_identities.exit_code IS 'The exit code of the command, when it is known.';
COMMENT ON COLUMN workload_identities.interactive IS 'The command runs in a terminal that its sponsor attaches to.';
COMMENT ON COLUMN workload_identities.remove_workspace IS 'The workspace was made for this workload and is deleted when the workload is over.';
COMMENT ON COLUMN workload_identities.output_tail IS 'The end of the workload''s output, kept for the record.';

CREATE INDEX workload_identities_workspace_id_idx ON workload_identities (workspace_id, created_at);
CREATE INDEX workload_identities_sponsor_id_idx ON workload_identities (sponsor_id, finished_at);
CREATE INDEX workload_identities_workspace_agent_id_idx ON workload_identities (workspace_agent_id) WHERE finished_at IS NULL;
CREATE UNIQUE INDEX workload_identities_user_id_idx ON workload_identities (user_id) WHERE user_id IS NOT NULL;

ALTER TYPE resource_type ADD VALUE IF NOT EXISTS 'workload_identity';
