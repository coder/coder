-- name: InsertChatHookDispatch :one
INSERT INTO chat_hook_dispatches (
	id,
	chat_id,
	event,
	turn_id,
	tool_use_id,
	tool_name,
	owner_id,
	workspace_id,
	started_at
) VALUES (
	@id::uuid,
	@chat_id::uuid,
	@event::text,
	sqlc.narg('turn_id')::uuid,
	sqlc.narg('tool_use_id')::text,
	sqlc.narg('tool_name')::text,
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
	decision_reason = sqlc.narg('decision_reason')::text,
	input_override = sqlc.narg('input_override')::jsonb,
	original_input = sqlc.narg('original_input')::jsonb,
	model_context = sqlc.narg('model_context')::text,
	user_message = sqlc.narg('user_message')::text,
	allowed_tools = sqlc.narg('allowed_tools')::jsonb,
	end_chat = sqlc.narg('end_chat')::boolean,
	error = sqlc.narg('error')::text
WHERE id = @id::uuid
	AND chat_id = @chat_id::uuid
	AND owner_id = @owner_id::uuid
RETURNING *;

-- name: DeleteOldChatHookDispatches :execrows
WITH deletable AS (
	SELECT id
	FROM chat_hook_dispatches
	WHERE started_at < @before_time::timestamptz
	ORDER BY started_at ASC
	LIMIT @limit_count::int
)
DELETE FROM chat_hook_dispatches
USING deletable
WHERE chat_hook_dispatches.id = deletable.id;

-- name: DeleteOrphanedChatHookDispatches :execrows
-- Dispatch rows have no foreign key on chat_id because create-time prompt
-- dispatches precede the chat row, so deleting or purging a chat leaves its
-- dispatch payloads behind. This sweep removes rows whose chat no longer
-- exists; before_time must trail NOW() by a grace period so in-flight
-- create-time dispatches are not swept before their chat row appears.
WITH deletable AS (
	SELECT chat_hook_dispatches.id
	FROM chat_hook_dispatches
	LEFT JOIN chats ON chats.id = chat_hook_dispatches.chat_id
	WHERE chats.id IS NULL
		AND chat_hook_dispatches.started_at < @before_time::timestamptz
	ORDER BY chat_hook_dispatches.started_at ASC
	LIMIT @limit_count::int
)
DELETE FROM chat_hook_dispatches
USING deletable
WHERE chat_hook_dispatches.id = deletable.id;

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

-- name: MarkChatHookDispatchEffectsApplied :exec
UPDATE chat_hook_dispatches
SET
	effects_applied_at = COALESCE(effects_applied_at, NOW())
WHERE
	chat_id = @chat_id::uuid
	AND event = 'pre_tool_use'
	AND id = ANY(@dispatch_ids::uuid[]);

-- name: GetChatHookDispatchDecision :one
SELECT
	*
FROM
	chat_hook_dispatches
WHERE
	chat_id = @chat_id::uuid
	AND event = 'pre_tool_use'
	AND tool_use_id = @tool_use_id::text
	AND tool_name = @tool_name::text
	AND (
		original_input = @tool_input::jsonb
		OR input_override = @tool_input::jsonb
	)
	AND turn_id IS NOT DISTINCT FROM sqlc.narg('turn_id')::uuid
	AND decision IS NOT NULL
	AND result IN ('ok', 'denied')
ORDER BY
	started_at DESC,
	id DESC
LIMIT 1;
