-- The new kind is not used in this migration: an enum value cannot be used in
-- the transaction that adds it.
ALTER TYPE chat_automation_kind ADD VALUE IF NOT EXISTS 'mcp_event';

CREATE TYPE chat_automation_event_delivery AS ENUM ('poll');

CREATE TYPE chat_automation_subscription_state AS ENUM ('active', 'terminated');

CREATE TYPE chat_automation_inbox_kind AS ENUM ('event', 'gap', 'terminated');

CREATE TABLE chat_automation_mcp_subscriptions (
    automation_id uuid PRIMARY KEY REFERENCES chat_automations (id) ON DELETE CASCADE,
    mcp_server_config_id uuid REFERENCES mcp_server_configs (id) ON DELETE SET NULL,
    event_name text NOT NULL,
    arguments jsonb NOT NULL DEFAULT '{}',
    delivery chat_automation_event_delivery NOT NULL,
    state chat_automation_subscription_state NOT NULL DEFAULT 'active',
    cursor text,
    next_poll_at timestamptz,
    drain_after timestamptz NOT NULL DEFAULT now(),
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chat_automation_mcp_subscriptions_poll_shape CHECK ((delivery = 'poll') = (next_poll_at IS NOT NULL))
);

COMMENT ON TABLE chat_automation_mcp_subscriptions IS 'The MCP event that an mcp_event chat automation subscribes to.';
COMMENT ON COLUMN chat_automation_mcp_subscriptions.mcp_server_config_id IS 'NULL after the MCP server config is deleted; the poll loop then terminates the subscription.';
COMMENT ON COLUMN chat_automation_mcp_subscriptions.cursor IS 'Opaque server cursor. NULL polls from now, or the event type does not replay.';
COMMENT ON COLUMN chat_automation_mcp_subscriptions.next_poll_at IS 'When the next poll is due. A claim moves it ahead as a lease.';
COMMENT ON COLUMN chat_automation_mcp_subscriptions.drain_after IS 'When pending inbox rows may next be sent to the chat. A claim moves it ahead as a lease.';

CREATE INDEX chat_automation_mcp_subscriptions_next_poll_at_idx ON chat_automation_mcp_subscriptions (next_poll_at) WHERE state = 'active';
CREATE INDEX chat_automation_mcp_subscriptions_drain_after_idx ON chat_automation_mcp_subscriptions (drain_after);
-- MCP server config deletion enforces ON DELETE SET NULL through this column.
CREATE INDEX chat_automation_mcp_subscriptions_config_idx ON chat_automation_mcp_subscriptions (mcp_server_config_id) WHERE mcp_server_config_id IS NOT NULL;

CREATE TABLE chat_automation_event_inbox (
    id bigserial PRIMARY KEY,
    automation_id uuid NOT NULL REFERENCES chat_automation_mcp_subscriptions (automation_id) ON DELETE CASCADE,
    kind chat_automation_inbox_kind NOT NULL,
    event_id text,
    event_name text,
    occurred_at timestamptz,
    data jsonb,
    detail text,
    received_at timestamptz NOT NULL DEFAULT now(),
    delivered_at timestamptz,
    CONSTRAINT chat_automation_event_inbox_shape CHECK ((kind = 'event') = (event_id IS NOT NULL AND occurred_at IS NOT NULL AND data IS NOT NULL))
);

COMMENT ON TABLE chat_automation_event_inbox IS 'Events and notices that every delivery mode appends, in arrival order, until a run sends them to the chat.';
COMMENT ON COLUMN chat_automation_event_inbox.detail IS 'The note of a gap row or the reason of a terminated row.';

-- Deduplicates an event within its subscription.
CREATE UNIQUE INDEX chat_automation_event_inbox_event_id_idx ON chat_automation_event_inbox (automation_id, event_id) WHERE event_id IS NOT NULL;
CREATE INDEX chat_automation_event_inbox_pending_idx ON chat_automation_event_inbox (automation_id, id) WHERE delivered_at IS NULL;
-- Serves the purge of delivered rows.
CREATE INDEX chat_automation_event_inbox_delivered_at_idx ON chat_automation_event_inbox (delivered_at) WHERE delivered_at IS NOT NULL;
