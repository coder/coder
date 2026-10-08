ALTER TYPE workspace_agent_context_body_kind ADD VALUE 'acp_harness';

CREATE TABLE agents_acp_sessions (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    chat_id UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    working_directory TEXT NOT NULL,
    harness_slug TEXT NOT NULL,
    harness_display_name TEXT NOT NULL,
    session_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, harness_slug, working_directory, session_id)
);

CREATE INDEX agents_acp_sessions_chat_updated_idx
    ON agents_acp_sessions (chat_id, updated_at DESC, id);
