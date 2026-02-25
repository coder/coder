CREATE TABLE workspace_git_events (
    id              UUID        DEFAULT gen_random_uuid() PRIMARY KEY,
    workspace_id    UUID        NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    agent_id        UUID        NOT NULL REFERENCES workspace_agents(id) ON DELETE CASCADE,
    owner_id        UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    -- Event data
    event_type      TEXT        NOT NULL,  -- 'session_start', 'commit', 'push', 'session_end'
    session_id      TEXT,                  -- links to agent session; nullable for orphan commits
    commit_sha      TEXT,                  -- for commit/push events
    commit_message  TEXT,                  -- for commit events
    branch          TEXT,
    repo_name       TEXT,                  -- repo basename or remote URL
    files_changed   TEXT[],                -- array of file paths
    agent_name      TEXT,                  -- 'claude-code', 'gemini-cli', 'unknown'

    -- AI Bridge join key (nullable — populated when AI Bridge is active)
    ai_bridge_interception_id UUID,        -- nullable FK to aibridge_interceptions(id)

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Performance indexes
CREATE INDEX idx_workspace_git_events_workspace ON workspace_git_events(workspace_id, created_at DESC);
CREATE INDEX idx_workspace_git_events_owner     ON workspace_git_events(owner_id, created_at DESC);
CREATE INDEX idx_workspace_git_events_session   ON workspace_git_events(session_id) WHERE session_id IS NOT NULL;
CREATE INDEX idx_workspace_git_events_org       ON workspace_git_events(organization_id, created_at DESC);
-- Cursor pagination index (matching AI Bridge pattern)
CREATE INDEX idx_workspace_git_events_pagination ON workspace_git_events(created_at DESC, id DESC);

COMMENT ON TABLE workspace_git_events IS 'Stores git events (commits, pushes, session boundaries) captured from AI coding sessions in workspaces.';
