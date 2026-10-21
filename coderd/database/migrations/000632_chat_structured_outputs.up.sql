CREATE TYPE chat_structured_output_status AS ENUM ('pending', 'succeeded', 'failed');

CREATE TABLE chat_structured_outputs (
    message_id bigint PRIMARY KEY REFERENCES chat_messages(id) ON DELETE CASCADE,
    chat_id uuid NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    status chat_structured_output_status NOT NULL DEFAULT 'pending',
    error_code text,
    request jsonb NOT NULL,
    value jsonb,
    rejected_calls integer NOT NULL DEFAULT 0,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT chat_structured_outputs_closed_check CHECK (
        (status = 'pending' AND error_code IS NULL)
        OR (status = 'succeeded' AND error_code IS NULL AND value IS NOT NULL)
        OR (status = 'failed' AND error_code IS NOT NULL)
    )
);

COMMENT ON TABLE chat_structured_outputs IS 'The structured output result of a chat user message that set response_format.';
COMMENT ON COLUMN chat_structured_outputs.request IS 'The response_format json_schema object: name, description and schema.';
COMMENT ON COLUMN chat_structured_outputs.value IS 'The last value the finalizer tool validated against the schema.';

-- At most one result is pending per chat; the close rule reads it by chat.
CREATE UNIQUE INDEX chat_structured_outputs_pending_idx ON chat_structured_outputs (chat_id) WHERE status = 'pending';
