-- name: InsertChatProjectTrigger :one
-- The organization comes from the project row, so the stored scope always
-- matches the project's organization.
INSERT INTO chat_project_triggers (
    project_id,
    organization_id,
    name,
    prompt,
    hashed_secret,
    created_by,
    source,
    events,
    repository,
    channel,
    text_pattern,
    text_pattern_is_regex,
    ignore_thread_replies,
    senders,
    continue_conversations,
    busy_behavior,
    schedule,
    next_run_at
)
SELECT
    chat_projects.id,
    chat_projects.organization_id,
    @name::text,
    @prompt::text,
    @hashed_secret::bytea,
    @created_by::uuid,
    @source::text,
    @events::text[],
    @repository::text,
    @channel::text,
    @text_pattern::text,
    @text_pattern_is_regex::boolean,
    @ignore_thread_replies::boolean,
    @senders::text[],
    @continue_conversations::boolean,
    @busy_behavior::text,
    @schedule::text,
    sqlc.narg('next_run_at')::timestamptz
FROM chat_projects
WHERE chat_projects.id = @project_id::uuid
RETURNING *;

-- name: GetChatProjectTriggerByID :one
SELECT *
FROM chat_project_triggers
WHERE id = @id::uuid;

-- name: GetChatProjectTriggersByProjectID :many
-- Oldest first, so a new trigger is added below the existing ones.
SELECT *
FROM chat_project_triggers
WHERE project_id = @project_id::uuid
ORDER BY created_at, id;

-- name: UpdateChatProjectTriggerByID :one
UPDATE chat_project_triggers
SET
    name = @name::text,
    prompt = @prompt::text,
    enabled = @enabled::boolean,
    events = @events::text[],
    repository = @repository::text,
    channel = @channel::text,
    text_pattern = @text_pattern::text,
    text_pattern_is_regex = @text_pattern_is_regex::boolean,
    ignore_thread_replies = @ignore_thread_replies::boolean,
    senders = @senders::text[],
    continue_conversations = @continue_conversations::boolean,
    busy_behavior = @busy_behavior::text,
    schedule = @schedule::text,
    next_run_at = sqlc.narg('next_run_at')::timestamptz,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateChatProjectTriggerLastTriggeredAt :exec
UPDATE chat_project_triggers
SET last_triggered_at = @last_triggered_at::timestamptz
WHERE id = @id::uuid;

-- name: GetDueChatProjectScheduleTriggers :many
-- Locks enabled schedule triggers that are due. Rows another replica has
-- locked are skipped, so each run is claimed once. Call it in a
-- transaction and move next_run_at forward before committing.
SELECT *
FROM chat_project_triggers
WHERE source = 'schedule'
    AND enabled
    AND next_run_at IS NOT NULL
    AND next_run_at <= @now::timestamptz
ORDER BY next_run_at
LIMIT @limit_opt::int
FOR UPDATE SKIP LOCKED;

-- name: UpdateChatProjectTriggerNextRunAt :exec
UPDATE chat_project_triggers
SET next_run_at = sqlc.narg('next_run_at')::timestamptz
WHERE id = @id::uuid;

-- name: DeleteChatProjectTriggerByID :exec
DELETE FROM chat_project_triggers
WHERE id = @id::uuid;

-- name: UpdateChatProjectTriggerSecret :one
UPDATE chat_project_triggers
SET
    hashed_secret = @hashed_secret::bytea,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: InsertChatProjectTriggerDelivery :one
INSERT INTO chat_project_trigger_deliveries (trigger_id, status, event, message, chat_id)
VALUES (@trigger_id::uuid, @status::text, @event::text, @message::text, sqlc.narg('chat_id')::uuid)
RETURNING *;

-- name: DeleteOldChatProjectTriggerDeliveries :exec
-- Keeps the most recent deliveries of a trigger.
DELETE FROM chat_project_trigger_deliveries
WHERE trigger_id = @trigger_id::uuid
    AND id NOT IN (
        SELECT id FROM chat_project_trigger_deliveries
        WHERE trigger_id = @trigger_id::uuid
        ORDER BY created_at DESC
        LIMIT @keep::int
    );

-- name: GetChatProjectTriggerDeliveries :many
SELECT *
FROM chat_project_trigger_deliveries
WHERE trigger_id = @trigger_id::uuid
ORDER BY created_at DESC
LIMIT @limit_opt::int;

-- name: GetLatestChatProjectTriggerDeliveriesByProjectID :many
SELECT DISTINCT ON (d.trigger_id) d.*
FROM chat_project_trigger_deliveries d
JOIN chat_project_triggers t ON t.id = d.trigger_id
WHERE t.project_id = @project_id::uuid
ORDER BY d.trigger_id, d.created_at DESC;

-- name: UpdateChatProjectTriggerSlackSigningSecret :one
-- An empty secret turns signature verification off.
UPDATE chat_project_triggers
SET
    slack_signing_secret = @slack_signing_secret::text,
    slack_signing_secret_key_id = sqlc.narg('slack_signing_secret_key_id')::text,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateChatProjectTriggerGitHubWebhookSecret :one
-- An empty secret turns signature verification off.
UPDATE chat_project_triggers
SET
    github_webhook_secret = @github_webhook_secret::text,
    github_webhook_secret_key_id = sqlc.narg('github_webhook_secret_key_id')::text,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: GetChatProjectTriggerConversation :one
SELECT *
FROM chat_project_trigger_conversations
WHERE trigger_id = @trigger_id::uuid
    AND conversation_key = @conversation_key::text;

-- name: UpsertChatProjectTriggerConversation :one
-- Points a conversation at a chat. A conversation whose chat was archived
-- moves to the new chat.
INSERT INTO chat_project_trigger_conversations (trigger_id, conversation_key, chat_id)
VALUES (@trigger_id::uuid, @conversation_key::text, @chat_id::uuid)
ON CONFLICT (trigger_id, conversation_key) DO UPDATE SET
    chat_id = EXCLUDED.chat_id,
    updated_at = now()
RETURNING *;

-- name: GetChatProjectTriggerConversationByChatID :one
-- Returns the external conversation a chat belongs to, such as the Slack
-- thread a triggered chat answers in.
SELECT
    c.trigger_id,
    c.conversation_key,
    c.chat_id,
    t.project_id,
    t.source
FROM chat_project_trigger_conversations c
JOIN chat_project_triggers t ON t.id = c.trigger_id
WHERE c.chat_id = @chat_id::uuid
ORDER BY c.updated_at DESC
LIMIT 1;

-- name: InsertChatProjectTriggerEvents :many
-- Claims event keys for a trigger. It returns only the keys that were not
-- already claimed, so a key missing from the result is a duplicate.
INSERT INTO chat_project_trigger_events (trigger_id, event_key)
SELECT @trigger_id::uuid, unnest(@event_keys::text[])
ON CONFLICT (trigger_id, event_key) DO NOTHING
RETURNING event_key;

-- name: DeleteChatProjectTriggerEvents :exec
-- Releases claimed event keys so a retried delivery is handled again.
DELETE FROM chat_project_trigger_events
WHERE trigger_id = @trigger_id::uuid
    AND event_key = ANY (@event_keys::text[]);

-- name: DeleteOldChatProjectTriggerEvents :exec
DELETE FROM chat_project_trigger_events
WHERE trigger_id = @trigger_id::uuid
    AND created_at < @before::timestamptz;
