-- name: InsertChatAutomation :one
INSERT INTO chat_automations (
    id,
    organization_id,
    owner_id,
    name,
    created_by_chat_id,
    kind,
    enabled,
    target_mode,
    target_chat_id,
    new_chat_model_config_id,
    reasoning_effort,
    when_busy,
    webhook_use,
    webhook_secret_hash,
    webhook_secret_version,
    prompt,
    schedule_cron,
    schedule_time_zone,
    schedule_next_run_at,
    created_at,
    updated_at
) VALUES (
    @id,
    @organization_id,
    @owner_id,
    @name,
    @created_by_chat_id,
    @kind,
    @enabled,
    @target_mode,
    @target_chat_id,
    @new_chat_model_config_id,
    @reasoning_effort,
    @when_busy,
    @webhook_use,
    -- A nil []byte is sent as an empty bytea, not NULL. Store NULL so
    -- automations without a secret satisfy chat_automations_kind_shape.
    NULLIF(@webhook_secret_hash::bytea, ''::bytea),
    @webhook_secret_version,
    @prompt,
    @schedule_cron,
    @schedule_time_zone,
    @schedule_next_run_at,
    @created_at,
    @updated_at
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

-- name: DeleteChatAutomationByID :exec
DELETE FROM
    chat_automations
WHERE
    id = @id::uuid;

-- name: GetChatAutomationsByIDsForUpdate :many
-- Locks the given automations in ascending id order so concurrent
-- lockers always acquire automation row locks in the same order.
-- Missing ids are not returned.
SELECT
    *
FROM
    chat_automations
WHERE
    id = ANY(@ids::uuid[])
ORDER BY
    id
FOR UPDATE;

-- name: GetChatAutomationsByOrganizationID :many
SELECT
    *
FROM
    chat_automations
WHERE
    organization_id = @organization_id::uuid
ORDER BY
    created_at DESC,
    id DESC;

-- name: CountChatAutomationsByOwnerID :one
-- Counts the automations owner_id owns across all organizations.
SELECT
    COUNT(*)
FROM
    chat_automations
WHERE
    owner_id = @owner_id::uuid;

-- name: UpdateChatAutomationByID :one
UPDATE
    chat_automations
SET
    name = @name,
    prompt = @prompt,
    target_chat_id = @target_chat_id,
    new_chat_model_config_id = @new_chat_model_config_id,
    reasoning_effort = @reasoning_effort,
    when_busy = @when_busy,
    schedule_cron = @schedule_cron,
    schedule_time_zone = @schedule_time_zone,
    schedule_revision = @schedule_revision,
    schedule_next_run_at = @schedule_next_run_at,
    updated_at = @updated_at
WHERE
    id = @id::uuid
RETURNING
    *;
