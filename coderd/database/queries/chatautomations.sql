-- name: InsertChatAutomation :one
INSERT INTO chat_automations (
    owner_id,
    name,
    description,
    icon,
    trigger_type,
    webhook_secret,
    cron_schedule,
    model_config_id,
    system_prompt,
    prompt_template,
    enabled,
    max_concurrent_runs
) VALUES (
    @owner_id,
    @name,
    @description,
    @icon,
    @trigger_type,
    sqlc.narg('webhook_secret'),
    sqlc.narg('cron_schedule'),
    @model_config_id,
    @system_prompt,
    @prompt_template,
    @enabled,
    @max_concurrent_runs
)
RETURNING
    *;

-- name: GetChatAutomationByID :one
SELECT
    *
FROM
    chat_automations
WHERE
    id = @id::uuid;

-- name: GetChatAutomationsByOwnerID :many
SELECT
    *
FROM
    chat_automations
WHERE
    owner_id = @owner_id::uuid
ORDER BY
    created_at DESC;

-- name: GetEnabledCronChatAutomations :many
SELECT
    *
FROM
    chat_automations
WHERE
    trigger_type = 'cron'
    AND enabled = TRUE;

-- name: UpdateChatAutomation :one
UPDATE
    chat_automations
SET
    name = @name,
    description = @description,
    icon = @icon,
    cron_schedule = sqlc.narg('cron_schedule'),
    system_prompt = @system_prompt,
    prompt_template = @prompt_template,
    model_config_id = @model_config_id,
    enabled = @enabled,
    max_concurrent_runs = @max_concurrent_runs,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: UpdateChatAutomationWebhookSecret :one
UPDATE
    chat_automations
SET
    webhook_secret = @webhook_secret,
    updated_at = NOW()
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: DeleteChatAutomation :exec
DELETE FROM chat_automations WHERE id = @id::uuid;

-- name: InsertChatAutomationRun :one
INSERT INTO chat_automation_runs (
    automation_id,
    trigger_payload,
    rendered_prompt
) VALUES (
    @automation_id,
    @trigger_payload,
    @rendered_prompt
)
RETURNING
    *;

-- name: UpdateChatAutomationRun :one
UPDATE
    chat_automation_runs
SET
    chat_id = COALESCE(sqlc.narg('chat_id'), chat_id),
    status = @status,
    error = sqlc.narg('error'),
    started_at = sqlc.narg('started_at'),
    completed_at = sqlc.narg('completed_at')
WHERE
    id = @id::uuid
RETURNING
    *;

-- name: GetChatAutomationRunsByAutomationID :many
SELECT
    *
FROM
    chat_automation_runs
WHERE
    automation_id = @automation_id::uuid
ORDER BY
    created_at DESC
LIMIT
    COALESCE(NULLIF(@limit_opt::int, 0), 50);

-- name: CountActiveChatAutomationRuns :one
SELECT
    COUNT(*)
FROM
    chat_automation_runs
WHERE
    automation_id = @automation_id::uuid
    AND status IN ('pending', 'running');
