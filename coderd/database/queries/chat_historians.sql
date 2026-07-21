-- name: GetChatHistorianClaims :many
SELECT
    hs.*,
    root.owner_id,
    EXISTS (
        SELECT 1
        FROM chat_messages message
        WHERE message.chat_id = root.id
          AND message.created_at >= NOW() - INTERVAL '24 hours'
    ) AS root_recent,
    historian.status AS historian_status,
    historian.archived AS historian_archived
FROM chat_historian_states hs
JOIN chats root ON root.id = hs.root_chat_id
LEFT JOIN chats historian ON historian.id = hs.historian_chat_id
WHERE hs.processing_history_version IS NOT NULL
   OR (
       historian.id IS NOT NULL
       AND historian.archived = false
       AND historian.status NOT IN (
           'waiting'::chat_status,
           'completed'::chat_status,
           'error'::chat_status
       )
   )
ORDER BY hs.processing_started_at ASC, hs.root_chat_id ASC;

-- name: GetChatHistorianCandidates :many
WITH candidates AS (
    SELECT DISTINCT ON (chat.owner_id)
        chat.*,
        COALESCE(hs.last_processed_history_version, 0)::bigint
            AS last_processed_history_version
    FROM chats_expanded chat
    LEFT JOIN chat_historian_states hs ON hs.root_chat_id = chat.id
    WHERE chat.parent_chat_id IS NULL
      AND chat.archived = false
      AND chat.status IN ('waiting'::chat_status, 'error'::chat_status)
      AND chat.updated_at <= NOW() - (INTERVAL '1 second' * @idle_seconds::int)
      AND EXISTS (
          SELECT 1
          FROM chat_messages message
          WHERE message.chat_id = chat.id
            AND message.created_at >= NOW() - INTERVAL '24 hours'
      )
      AND chat.history_version > COALESCE(hs.last_processed_history_version, 0)
      AND hs.processing_history_version IS NULL
      AND COALESCE(chat.labels->>'slack_shared', 'false') <> 'true'
      AND chat.group_acl = '{}'::jsonb
      AND NOT EXISTS (
          SELECT 1
          FROM jsonb_object_keys(chat.user_acl) acl_user_id
          WHERE acl_user_id <> chat.owner_id::text
      )
    ORDER BY chat.owner_id, chat.updated_at ASC, chat.id ASC
)
SELECT *
FROM candidates
ORDER BY updated_at ASC, id ASC
LIMIT @limit_count::int;

-- name: ClaimChatHistorianHistory :one
INSERT INTO chat_historian_states (
    root_chat_id,
    processing_history_version,
    processing_started_at,
    dispatch_id
) VALUES (
    @root_chat_id::uuid,
    @processing_history_version::bigint,
    @processing_started_at::timestamptz,
    @dispatch_id::uuid
)
ON CONFLICT (root_chat_id) DO UPDATE
SET processing_history_version = EXCLUDED.processing_history_version,
    processing_started_at = EXCLUDED.processing_started_at,
    dispatch_id = EXCLUDED.dispatch_id,
    dispatched_at = NULL,
    updated_at = now()
WHERE chat_historian_states.processing_history_version IS NULL
  AND chat_historian_states.last_processed_history_version
      < EXCLUDED.processing_history_version
RETURNING *;

-- name: SetChatHistorianChild :one
UPDATE chat_historian_states
SET historian_chat_id = @historian_chat_id::uuid,
    updated_at = now()
WHERE root_chat_id = @root_chat_id::uuid
RETURNING *;

-- name: MarkChatHistorianDispatched :one
UPDATE chat_historian_states
SET dispatched_at = now(),
    updated_at = now()
WHERE root_chat_id = @root_chat_id::uuid
  AND dispatch_id = @dispatch_id::uuid
  AND processing_history_version = @processing_history_version::bigint
RETURNING *;

-- name: CompleteChatHistorianHistory :one
UPDATE chat_historian_states
SET last_processed_history_version = processing_history_version,
    processing_history_version = NULL,
    processing_started_at = NULL,
    dispatch_id = NULL,
    dispatched_at = NULL,
    updated_at = now()
WHERE root_chat_id = @root_chat_id::uuid
  AND dispatch_id = @dispatch_id::uuid
  AND dispatched_at IS NOT NULL
RETURNING *;

-- name: ClearChatHistorianClaim :one
UPDATE chat_historian_states
SET processing_history_version = NULL,
    processing_started_at = NULL,
    dispatch_id = NULL,
    dispatched_at = NULL,
    updated_at = now()
WHERE root_chat_id = @root_chat_id::uuid
  AND dispatch_id = @dispatch_id::uuid
RETURNING *;

-- name: AdvanceChatHistorianHistory :one
INSERT INTO chat_historian_states (
    root_chat_id,
    last_processed_history_version
) VALUES (
    @root_chat_id::uuid,
    @history_version::bigint
)
ON CONFLICT (root_chat_id) DO UPDATE
SET last_processed_history_version = EXCLUDED.last_processed_history_version,
    updated_at = now()
WHERE chat_historian_states.processing_history_version IS NULL
  AND chat_historian_states.last_processed_history_version
      < EXCLUDED.last_processed_history_version
RETURNING *;

-- name: GetChatMessagesForHistorian :many
SELECT *
FROM chat_messages
WHERE chat_id = @chat_id::uuid
  AND revision > @after_history_version::bigint
  AND revision <= @through_history_version::bigint
  AND deleted = false
  AND compressed = false
  AND visibility IN ('user'::chat_message_visibility, 'both'::chat_message_visibility)
  AND role IN ('user'::chat_message_role, 'assistant'::chat_message_role)
ORDER BY revision ASC, id ASC;

-- name: GetLatestChatUserAPIKeyForHistorian :one
SELECT api_key_id
FROM chat_messages
WHERE chat_id = @chat_id::uuid
  AND revision <= @through_history_version::bigint
  AND deleted = false
  AND role = 'user'::chat_message_role
  AND api_key_id IS NOT NULL
  AND api_key_id <> ''
ORDER BY revision DESC, id DESC
LIMIT 1;
