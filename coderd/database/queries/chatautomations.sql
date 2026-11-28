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
    new_chat_model_config_id,
    project_id,
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
    sqlc.narg('new_chat_model_config_id')::uuid,
    sqlc.narg('project_id')::uuid,
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
-- The all-zero owner_id means every owner.
SELECT *
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
