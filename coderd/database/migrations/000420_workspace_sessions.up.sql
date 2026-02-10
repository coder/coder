CREATE TABLE workspace_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id uuid REFERENCES workspace_agents(id) ON DELETE SET NULL,
    ip inet NOT NULL,
    client_hostname text,
    short_description text,
    started_at timestamp with time zone NOT NULL,
    ended_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_sessions_workspace ON workspace_sessions (workspace_id, started_at DESC);
CREATE INDEX idx_workspace_sessions_lookup ON workspace_sessions (workspace_id, ip, started_at);

ALTER TABLE connection_logs
    ADD COLUMN session_id uuid REFERENCES workspace_sessions(id) ON DELETE SET NULL,
    ADD COLUMN client_hostname text,
    ADD COLUMN short_description text;

CREATE INDEX idx_connection_logs_session ON connection_logs (session_id) WHERE session_id IS NOT NULL;
