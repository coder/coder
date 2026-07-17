-- chat_tool_call_executions is a durable ledger of execute tool call
-- lifecycles. Retried task attempts claim the ledger row instead of
-- dispatching again, and later attempts re-attach, observe, or
-- reconcile instead of silently starting another process. The ledger
-- cannot dedup a dispatch whose response was lost in transit; such a
-- call resolves to unknown rather than dispatching again.
CREATE TYPE chat_tool_call_execution_status AS ENUM (
    'reserved',
    'starting',
    'running',
    'exited',
    'detached',
    'cancel_requested',
    'canceled',
    'unknown',
    'no_effect'
);

-- Supports the composite lineage foreign key below, which requires a
-- unique key over the referenced columns. Redundant with the primary
-- key for lookups, but it lets the ledger enforce that a lineage row
-- can only reference a message inside its own chat.
CREATE UNIQUE INDEX idx_chat_messages_chat_id_id ON chat_messages (chat_id, id);

CREATE TABLE chat_tool_call_executions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id               UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    assistant_message_id  BIGINT NOT NULL,
    tool_call_id          TEXT NOT NULL,
    status                chat_tool_call_execution_status NOT NULL DEFAULT 'reserved',
    input_sha256          TEXT NOT NULL,
    command               TEXT NOT NULL DEFAULT '',
    background            BOOLEAN NOT NULL DEFAULT false,
    timeout_secs          BIGINT NOT NULL DEFAULT 0,
    claim_epoch           BIGINT NOT NULL DEFAULT 0,
    claimed_at            TIMESTAMPTZ,
    workspace_agent_id    UUID REFERENCES workspace_agents(id) ON DELETE SET NULL,
    process_id            TEXT,
    cancel_signal_sent_at TIMESTAMPTZ,
    result_committed_at   TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at            TIMESTAMPTZ,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (chat_id, assistant_message_id, tool_call_id),
    -- The composite reference rejects lineage rows whose assistant
    -- message belongs to a different chat, so a caller authorized
    -- for one chat cannot attach ledger rows to another chat's
    -- messages.
    FOREIGN KEY (chat_id, assistant_message_id) REFERENCES chat_messages (chat_id, id) ON DELETE CASCADE,
    -- Process identity is recorded atomically: a row either has a
    -- dispatched process with its start time or neither.
    CHECK ((process_id IS NULL) = (started_at IS NULL))
);

COMMENT ON COLUMN chat_tool_call_executions.id IS 'Stable execution identity, generated at intent creation.';
COMMENT ON COLUMN chat_tool_call_executions.assistant_message_id IS 'Lineage: the assistant message that issued the tool call. Provider tool call IDs may repeat across regenerated messages, so identity is (chat_id, assistant_message_id, tool_call_id).';
COMMENT ON COLUMN chat_tool_call_executions.input_sha256 IS 'SHA-256 of the persisted tool input, asserted at claim time to catch stale lineage.';
COMMENT ON COLUMN chat_tool_call_executions.command IS 'Recorded at claim time for diagnostics only; never used for deduplication.';
COMMENT ON COLUMN chat_tool_call_executions.timeout_secs IS 'The clamped foreground tool timeout, recorded at claim time. Zero for background executions, which have no completion deadline.';
COMMENT ON COLUMN chat_tool_call_executions.claim_epoch IS 'Incremented on every claim. Guards process-identity writes so a superseded claimer cannot overwrite the current claim.';
COMMENT ON COLUMN chat_tool_call_executions.cancel_signal_sent_at IS 'Set when an interrupt delivered a kill signal whose effect was not yet confirmed.';
COMMENT ON COLUMN chat_tool_call_executions.result_committed_at IS 'Set in the transaction that commits the tool result message (real or synthetic). History-delete transitions also re-stamp it to the edit time to restart the sweep''s give-up bound. Orthogonal to status, which keeps lifecycle truth.';

-- Serves the committed-row purge, which windows on the result
-- commit time so late-resolving executions keep their full
-- diagnostic retention.
CREATE INDEX idx_chat_tool_call_executions_committed_at ON chat_tool_call_executions (result_committed_at) WHERE result_committed_at IS NOT NULL;

-- Serves the workspace-agent FK's ON DELETE SET NULL check when
-- agents are deleted. Partial: most retained rows have a NULL agent
-- after their workspace is cleaned up.
CREATE INDEX idx_chat_tool_call_executions_workspace_agent_id ON chat_tool_call_executions (workspace_agent_id) WHERE workspace_agent_id IS NOT NULL;

-- Serves the periodic sweep that retries stalled cancellations,
-- which claims cancel_requested rows ordered by updated_at.
-- Partial: cancel_requested rows are rare and transient.
CREATE INDEX idx_chat_tool_call_executions_cancel_sweep ON chat_tool_call_executions (updated_at) WHERE status = 'cancel_requested';

-- Serves the abandoned-row purge, which reaps uncommitted rows idle
-- past a long horizon. Uncommitted rows are rare relative to the
-- committed population, so without this partial index the purge
-- seq-scans the whole table every tick.
CREATE INDEX idx_chat_tool_call_executions_uncommitted ON chat_tool_call_executions (updated_at) WHERE result_committed_at IS NULL;
