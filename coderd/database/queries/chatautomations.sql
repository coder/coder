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
