-- Create-time prompt dispatches precede the chat row, so chat_id has no
-- foreign key.
CREATE TABLE chat_hook_dispatches (
    id uuid PRIMARY KEY,
    chat_id uuid NOT NULL,
    event text NOT NULL,
    turn_id uuid,
    tool_use_id text,
    owner_id uuid NOT NULL,
    workspace_id uuid,
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    result text NOT NULL DEFAULT 'pending',
    http_status integer,
    decision text,
    input_override jsonb,
    original_input jsonb,
    model_context text,
    user_message text,
    allowed_tools jsonb,
    end_chat boolean,
    error text,
    decision_reason text,
    effects_applied_at timestamptz,
    tool_name text
);

COMMENT ON TABLE chat_hook_dispatches IS 'Lifecycle hook attempts keyed by dispatch_id (JWT jti).';

CREATE INDEX idx_chat_hook_dispatches_chat_id ON chat_hook_dispatches (chat_id);
CREATE INDEX idx_chat_hook_dispatches_started_at ON chat_hook_dispatches (started_at);
