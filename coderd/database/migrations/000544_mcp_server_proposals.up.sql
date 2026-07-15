CREATE TABLE mcp_server_proposals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id uuid NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    requester_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    channel text NOT NULL,
    thread_ts text NOT NULL,
    message_ts text NOT NULL,
    request jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    mcp_server_config_id uuid REFERENCES mcp_server_configs (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    CONSTRAINT mcp_server_proposals_status_check CHECK (status = ANY (ARRAY['pending'::text, 'accepted'::text, 'rejected'::text]))
);

COMMENT ON TABLE mcp_server_proposals IS 'Pending MCP server proposals created by the propose_mcp_server chat tool for Slack-bound chats. A proposal is accepted or rejected by the requesting user through the dashboard, or cancelled from Slack.';
COMMENT ON COLUMN mcp_server_proposals.requester_id IS 'Coder user the proposing Slack sender resolved to, recorded at proposal creation. Only this user may accept, reject, or cancel the proposal.';
COMMENT ON COLUMN mcp_server_proposals.request IS 'The proposed MCP server config as JSON. May contain secrets (api key values, custom headers); never returned verbatim through the API.';
COMMENT ON COLUMN mcp_server_proposals.mcp_server_config_id IS 'The MCP server config created when the proposal was accepted. NULL while pending or rejected.';

CREATE INDEX idx_mcp_server_proposals_chat_id ON mcp_server_proposals (chat_id);
