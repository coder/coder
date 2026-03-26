-- name: InsertAutomationTrigger :one
INSERT INTO automation_triggers (
    automation_id,
    type,
    webhook_secret,
    webhook_secret_key_id,
    cron_schedule,
    filter,
    label_paths
) VALUES (
    @automation_id::uuid,
    @type::text,
    sqlc.narg('webhook_secret')::text,
    sqlc.narg('webhook_secret_key_id')::text,
    sqlc.narg('cron_schedule')::text,
    sqlc.narg('filter')::jsonb,
    sqlc.narg('label_paths')::jsonb
) RETURNING *;

-- name: GetAutomationTriggerByID :one
SELECT * FROM automation_triggers WHERE id = @id::uuid;

-- name: GetAutomationTriggersByAutomationID :many
SELECT * FROM automation_triggers
WHERE automation_id = @automation_id::uuid
ORDER BY created_at ASC;

-- name: UpdateAutomationTrigger :one
UPDATE automation_triggers SET
    cron_schedule = sqlc.narg('cron_schedule')::text,
    filter = sqlc.narg('filter')::jsonb,
    label_paths = sqlc.narg('label_paths')::jsonb,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateAutomationTriggerWebhookSecret :one
UPDATE automation_triggers SET
    webhook_secret = sqlc.narg('webhook_secret')::text,
    webhook_secret_key_id = sqlc.narg('webhook_secret_key_id')::text,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteAutomationTriggerByID :exec
DELETE FROM automation_triggers WHERE id = @id::uuid;

-- name: GetActiveCronTriggers :many
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
FROM automation_triggers t
JOIN automations a ON a.id = t.automation_id
WHERE t.type = 'cron'
  AND t.cron_schedule IS NOT NULL
  AND a.status IN ('active', 'preview');

-- name: UpdateAutomationTriggerLastTriggeredAt :exec
UPDATE automation_triggers
SET last_triggered_at = @last_triggered_at::timestamptz
WHERE id = @id::uuid;
