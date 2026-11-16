CREATE TABLE workspace_agent_relays (
    workspace_agent_id uuid PRIMARY KEY REFERENCES workspace_agents (id) ON DELETE CASCADE,
    public_key bytea NOT NULL,
    generation bigint NOT NULL DEFAULT 1,
    created_at timestamp with time zone NOT NULL,
    renewed_at timestamp with time zone NOT NULL
);

COMMENT ON TABLE workspace_agent_relays IS 'Which instance of a workspace daemon may relay requests to the control plane on behalf of what runs in its workspace. One instance holds the relay at a time, for as long as it keeps renewing its lease.';
COMMENT ON COLUMN workspace_agent_relays.public_key IS 'The Ed25519 public key of the daemon instance that holds the relay. The instance made the key when it started and keeps the private half in memory, so a copy of the daemon''s token is not enough to renew.';
COMMENT ON COLUMN workspace_agent_relays.generation IS 'How many daemon instances have held the relay. It goes up when another instance takes over.';
COMMENT ON COLUMN workspace_agent_relays.renewed_at IS 'When the holder last obtained a lease. Another instance can take over once the holder has not renewed for longer than a lease lasts.';
