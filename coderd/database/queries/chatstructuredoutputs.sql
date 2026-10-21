-- name: InsertChatStructuredOutput :one
INSERT INTO chat_structured_outputs (message_id, chat_id, request)
VALUES (@message_id::bigint, @chat_id::uuid, @request::jsonb)
RETURNING *;

-- name: GetChatStructuredOutput :one
SELECT * FROM chat_structured_outputs
WHERE chat_id = @chat_id::uuid AND message_id = @message_id::bigint;

-- name: GetPendingChatStructuredOutput :one
SELECT * FROM chat_structured_outputs
WHERE chat_id = @chat_id::uuid AND status = 'pending';

-- name: UpdatePendingChatStructuredOutput :one
-- Writes only a pending result, so a closed result never changes.
UPDATE chat_structured_outputs
SET
    status = @status::chat_structured_output_status,
    error_code = sqlc.narg('error_code')::text,
    value = sqlc.narg('value')::jsonb,
    rejected_calls = @rejected_calls::integer,
    updated_at = now()
WHERE chat_id = @chat_id::uuid AND message_id = @message_id::bigint AND status = 'pending'
RETURNING *;

-- name: HasChatUserMessageAfter :one
-- Reports whether a client-visible user message follows after_id, which
-- means a newer prompt started a turn.
SELECT EXISTS (
    SELECT 1 FROM chat_messages
    WHERE chat_id = @chat_id::uuid
        AND id > @after_id::bigint
        AND role = 'user'
        AND visibility IN ('user', 'both')
        AND deleted = false
)::boolean;
