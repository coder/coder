CREATE TYPE chat_execution_step_operation AS ENUM (
    'model',
    'local_tool_batch',
    'compaction'
);

CREATE TYPE chat_execution_step_outcome AS ENUM (
    'completed',
    'interrupted'
);

CREATE TABLE chat_execution_steps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id uuid REFERENCES chats(id) ON DELETE SET NULL,
    history_version bigint NOT NULL,
    generation_attempt bigint NOT NULL,
    operation chat_execution_step_operation NOT NULL,
    outcome chat_execution_step_outcome NOT NULL,
    runtime_ms bigint NOT NULL CONSTRAINT chat_execution_steps_runtime_ms_nonnegative CHECK (runtime_ms >= 0),
    recorded_at timestamp with time zone NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_chat_execution_steps_episode
    ON chat_execution_steps (chat_id, history_version, generation_attempt)
    WHERE chat_id IS NOT NULL;

CREATE INDEX idx_chat_execution_steps_recorded_at
    ON chat_execution_steps (recorded_at);

CREATE INDEX idx_chat_execution_steps_chat_id
    ON chat_execution_steps (chat_id)
    WHERE chat_id IS NOT NULL;

ALTER TABLE chat_messages
    ADD COLUMN execution_step_id uuid REFERENCES chat_execution_steps(id);

CREATE INDEX idx_chat_messages_execution_step_id
    ON chat_messages (execution_step_id)
    WHERE execution_step_id IS NOT NULL;

COMMENT ON TABLE chat_execution_steps IS
    'De-identified billable execution runtime. chat_id is cleared when its chat is hard-deleted.';

COMMENT ON COLUMN chat_messages.execution_step_id IS
    'Associates a generated message batch with the execution step that produced it.';

COMMENT ON COLUMN chat_messages.runtime_ms IS
    'Deprecated rolling-upgrade compatibility column. New runtime is stored on chat_execution_steps.';
