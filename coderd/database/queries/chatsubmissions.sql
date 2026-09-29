-- name: InsertChatSubmission :one
INSERT INTO chat_submissions
    (id, organization_id, actor_id, owner_id, request_id, input_digest, kind, chat_id, settings)
VALUES
    (@id, @organization_id, @actor_id, @owner_id, @request_id, @input_digest, @kind, @chat_id, @settings)
ON CONFLICT (organization_id, actor_id, request_id) DO NOTHING
RETURNING *;

-- name: GetChatSubmission :one
SELECT * FROM chat_submissions
WHERE organization_id = @organization_id AND actor_id = @actor_id AND request_id = @request_id;

-- name: CompleteChatSubmission :one
UPDATE chat_submissions
SET state = 'accepted',
    message_id = sqlc.narg('message_id')::bigint,
    queued_message_id = sqlc.narg('queued_message_id')::bigint
WHERE organization_id = @organization_id AND actor_id = @actor_id AND request_id = @request_id AND state = 'reserved'
RETURNING *;

-- name: FinishChatSubmission :one
UPDATE chat_submissions SET state = @state::text, error = @error
WHERE organization_id = @organization_id AND actor_id = @actor_id
    AND request_id = @request_id AND id = @id AND state = 'reserved'
    AND @state::text IN ('rejected', 'uncertain')
RETURNING *;

-- name: GetLatestChatSubmissionSettings :one
-- Use the actual newest visible user turn, including a promoted queued turn.
-- A later legacy message must not inherit a previous exact-settings snapshot.
SELECT COALESCE(s.settings, 'null'::jsonb)::jsonb AS settings
FROM chat_messages m
LEFT JOIN chat_submissions s ON s.chat_id = m.chat_id
    AND s.state = 'accepted'
    AND (s.message_id = m.id OR s.queued_message_id = m.queued_message_id)
WHERE m.chat_id = @chat_id AND m.role = 'user' AND m.deleted = false
    AND m.visibility IN ('user', 'both')
ORDER BY m.id DESC
LIMIT 1;
