-- chat_tool_call_executions tracks in-flight execute tool calls so
-- that a retried task attempt can re-attach to a process started by
-- a previous attempt instead of spawning a duplicate.
CREATE TABLE chat_tool_call_executions (
    chat_id            UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    tool_call_id       TEXT        NOT NULL,
    workspace_agent_id UUID        REFERENCES workspace_agents(id) ON DELETE SET NULL,
    process_id         TEXT,
    command            TEXT        NOT NULL,
    background         BOOLEAN     NOT NULL DEFAULT false,
    timeout_secs       BIGINT      NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at         TIMESTAMPTZ,
    PRIMARY KEY (chat_id, tool_call_id)
);

COMMENT ON COLUMN chat_tool_call_executions.process_id IS 'NULL means the row was reserved but the process handle was never recorded; set together with started_at once the process starts.';
COMMENT ON COLUMN chat_tool_call_executions.command IS 'Recorded for mismatch diagnostics only; never used for deduplication.';
COMMENT ON COLUMN chat_tool_call_executions.timeout_secs IS 'The clamped tool timeout at reserve time.';

CREATE INDEX idx_chat_tool_call_executions_created_at ON chat_tool_call_executions (created_at);
