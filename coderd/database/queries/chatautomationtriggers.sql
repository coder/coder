-- name: InsertChatAutomationTrigger :one
INSERT INTO chat_automation_triggers (
    id,
    automation_id,
    type,
    webhook_secret,
    webhook_secret_key_id,
    cron_schedule,
    filter,
    label_paths,
    created_at,
    updated_at
) VALUES (
    @id::uuid,
    @automation_id::uuid,
    @type::chat_automation_trigger_type,
    sqlc.narg('webhook_secret')::text,
    sqlc.narg('webhook_secret_key_id')::text,
    sqlc.narg('cron_schedule')::text,
    sqlc.narg('filter')::jsonb,
    sqlc.narg('label_paths')::jsonb,
    @created_at::timestamptz,
    @updated_at::timestamptz
) RETURNING *;

-- name: GetChatAutomationTriggerByID :one
SELECT * FROM chat_automation_triggers WHERE id = @id::uuid;

-- name: GetChatAutomationTriggersByAutomationID :many
SELECT * FROM chat_automation_triggers
WHERE automation_id = @automation_id::uuid
ORDER BY created_at ASC;

-- name: UpdateChatAutomationTrigger :one
UPDATE chat_automation_triggers SET
    cron_schedule = COALESCE(sqlc.narg('cron_schedule'), cron_schedule),
    filter = COALESCE(sqlc.narg('filter'), filter),
    label_paths = COALESCE(sqlc.narg('label_paths'), label_paths),
    updated_at = @updated_at::timestamptz
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateChatAutomationTriggerWebhookSecret :one
UPDATE chat_automation_triggers SET
    webhook_secret = sqlc.narg('webhook_secret')::text,
    webhook_secret_key_id = sqlc.narg('webhook_secret_key_id')::text,
    updated_at = @updated_at::timestamptz
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatAutomationTriggerByID :exec
DELETE FROM chat_automation_triggers WHERE id = @id::uuid;

-- name: GetActiveChatAutomationCronTriggers :many
-- Returns all cron triggers whose parent automation is active or in
-- preview mode. The scheduler uses this to evaluate which triggers
-- are due.
SELECT
    t.id,
    t.automation_id,
    t.type,
    t.cron_schedule,
    t.filter,
    t.label_paths,
    t.last_triggered_at,
    t.created_at,
    t.updated_at,
    a.status AS automation_status,
    a.owner_id AS automation_owner_id,
    a.instructions AS automation_instructions,
    a.name AS automation_name,
    a.organization_id AS automation_organization_id,
    a.model_config_id AS automation_model_config_id,
    a.mcp_server_ids AS automation_mcp_server_ids,
    a.allowed_tools AS automation_allowed_tools,
    a.max_chat_creates_per_hour AS automation_max_chat_creates_per_hour,
    a.max_messages_per_hour AS automation_max_messages_per_hour
FROM chat_automation_triggers t
JOIN chat_automations a ON a.id = t.automation_id
WHERE t.type = 'cron'
  AND t.cron_schedule IS NOT NULL
  AND a.status IN ('active', 'preview');

-- name: UpdateChatAutomationTriggerLastTriggeredAt :exec
UPDATE chat_automation_triggers
SET last_triggered_at = @last_triggered_at::timestamptz
WHERE id = @id::uuid;
