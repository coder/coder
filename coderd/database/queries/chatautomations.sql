-- name: InsertChatAutomation :one
INSERT INTO chat_automations (
    id,
    organization_id,
    owner_id,
    name,
    created_by_chat_id,
    kind,
    enabled,
    prompt,
    target_mode,
    target_chat_id,
    when_busy,
    new_chat_model_config_id,
    reasoning_effort,
    project_id,
    webhook_auth,
    webhook_secret,
    webhook_secret_key_id,
    schedule_cron,
    schedule_time_zone,
    schedule_next_run_at
)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @owner_id::uuid,
    @name::text,
    sqlc.narg('created_by_chat_id')::uuid,
    @kind::chat_automation_kind,
    @enabled::boolean,
    @prompt::text,
    @target_mode::chat_automation_target_mode,
    sqlc.narg('target_chat_id')::uuid,
    sqlc.narg('when_busy')::chat_automation_when_busy,
    sqlc.narg('new_chat_model_config_id')::uuid,
    sqlc.narg('reasoning_effort')::chat_reasoning_effort,
    sqlc.narg('project_id')::uuid,
    sqlc.narg('webhook_auth')::chat_automation_webhook_auth,
    sqlc.narg('webhook_secret')::text,
    sqlc.narg('webhook_secret_key_id')::text,
    sqlc.narg('schedule_cron')::text,
    sqlc.narg('schedule_time_zone')::text,
    sqlc.narg('schedule_next_run_at')::timestamptz
)
RETURNING *;

-- name: GetChatAutomationByID :one
SELECT *
FROM chat_automations
WHERE id = @id::uuid;

-- name: GetChatAutomationsByOrganizationID :many
-- The all-zero owner_id means every owner. The webhook secret and its key
-- ID are left out, so a list never loads or decrypts a secret.
SELECT
    id,
    organization_id,
    owner_id,
    name,
    created_by_chat_id,
    kind,
    enabled,
    prompt,
    target_mode,
    target_chat_id,
    when_busy,
    new_chat_model_config_id,
    reasoning_effort,
    project_id,
    webhook_auth,
    webhook_secret_version,
    schedule_cron,
    schedule_time_zone,
    schedule_next_run_at,
    schedule_revision,
    last_run_at,
    last_run_outcome,
    last_run_reason,
    last_run_chat_id,
    last_run_unaccepted_count,
    created_at,
    updated_at
FROM
    chat_automations
WHERE
    organization_id = @organization_id::uuid
    AND CASE
        WHEN @owner_id::uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN owner_id = @owner_id::uuid
        ELSE true
    END
    -- Authorize Filter clause will be injected below in GetAuthorizedChatAutomationsByOrganizationID
    -- @authorize_filter
ORDER BY
    created_at DESC,
    id DESC;

-- name: CountChatAutomationsByOwnerID :one
-- Counts the automations of one owner across all organizations.
SELECT COUNT(*)::bigint
FROM chat_automations
WHERE owner_id = @owner_id::uuid;

-- name: DeleteChatAutomationByID :exec
DELETE FROM chat_automations
WHERE id = @id::uuid;

-- name: GetChatIDsWithQueuedMessagesByAutomationID :many
-- Returns the chats that hold messages that the automation queued, in the
-- order in which a delete locks them.
SELECT DISTINCT chat_id
FROM chat_queued_messages
WHERE automation_id = @automation_id::uuid
ORDER BY chat_id;

-- name: DeleteChatQueuedMessagesByAutomationID :exec
-- Deletes the messages that the automation queued in one chat. The caller
-- holds the lock of the chat.
DELETE FROM chat_queued_messages
WHERE automation_id = @automation_id::uuid
  AND chat_id = @chat_id::uuid;
