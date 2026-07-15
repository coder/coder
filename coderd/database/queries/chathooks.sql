-- name: InsertChatHookDispatch :one
INSERT INTO chat_hook_dispatches (
	id,
	chat_id,
	event,
	turn_id,
	tool_use_id,
	owner_id,
	workspace_id,
	started_at
) VALUES (
	@id::uuid,
	@chat_id::uuid,
	@event::text,
	sqlc.narg('turn_id')::uuid,
	sqlc.narg('tool_use_id')::text,
	@owner_id::uuid,
	sqlc.narg('workspace_id')::uuid,
	@started_at::timestamptz
)
RETURNING *;

-- name: FinalizeChatHookDispatch :one
UPDATE chat_hook_dispatches
SET
	finished_at = @finished_at::timestamptz,
	result = @result::text,
	http_status = sqlc.narg('http_status')::integer,
	decision = sqlc.narg('decision')::text,
	input_override = sqlc.narg('input_override')::jsonb,
	original_input = sqlc.narg('original_input')::jsonb,
	model_context = sqlc.narg('model_context')::text,
	user_message = sqlc.narg('user_message')::text,
	allowed_tools = sqlc.narg('allowed_tools')::jsonb,
	end_chat = sqlc.narg('end_chat')::boolean,
	error = sqlc.narg('error')::text
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateChatHookAllowedTools :exec
UPDATE chats
SET
	hook_allowed_tools = sqlc.narg('hook_allowed_tools')::jsonb,
	updated_at = NOW()
WHERE id = @id::uuid;

-- name: ListChatHookDispatchesByChatID :many
SELECT
	*
FROM
	chat_hook_dispatches
WHERE
	chat_id = @chat_id::uuid
ORDER BY
	started_at ASC,
	id ASC;
