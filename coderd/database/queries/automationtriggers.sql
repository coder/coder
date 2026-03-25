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
