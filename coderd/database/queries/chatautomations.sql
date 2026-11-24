-- name: InsertChatAutomation :one
-- The shared lock on the owner row makes a concurrent soft delete of the owner
-- wait for this transaction, so its cleanup trigger then deletes the new row.
-- The insert trigger refuses an owner that is already deleted.
WITH owner AS (
    SELECT id
    FROM users
    WHERE id = @owner_id::uuid
    FOR SHARE
)
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
SELECT
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    owner.id,
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
FROM owner
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

-- name: ClaimDueChatAutomationSchedules :many
-- Locks enabled schedules that are due. SKIP LOCKED lets every replica claim
-- a different set; the caller advances each row in the same transaction.
SELECT *
FROM chat_automations
WHERE
    kind = 'schedule'
    AND enabled
    AND schedule_next_run_at <= @now::timestamptz
ORDER BY schedule_next_run_at
LIMIT @limit_count::int
FOR UPDATE SKIP LOCKED;

-- name: AdvanceChatAutomationNextRunAt :exec
UPDATE chat_automations
SET schedule_next_run_at = sqlc.narg('schedule_next_run_at')::timestamptz
WHERE id = @id::uuid;

-- name: UpdateChatAutomationLastRun :execrows
-- updated_at tracks changes by people, so a run leaves it alone. A run that
-- was not accepted is recorded only while the automation is still enabled and
-- unchanged since the run read it, so a disable or change that commits first
-- wins. An accepted run sent its message, so it is always recorded.
UPDATE chat_automations
SET
    last_run_at = @last_run_at::timestamptz,
    last_run_outcome = @last_run_outcome::chat_automation_run_outcome,
    last_run_reason = sqlc.narg('last_run_reason')::text,
    last_run_chat_id = sqlc.narg('last_run_chat_id')::uuid,
    last_run_unaccepted_count = CASE
        WHEN @last_run_outcome::chat_automation_run_outcome = 'accepted' THEN 0
        ELSE last_run_unaccepted_count + 1
    END
WHERE
    id = @id::uuid
    AND (
        @last_run_outcome::chat_automation_run_outcome = 'accepted'
        OR (enabled AND updated_at = @read_updated_at::timestamptz)
    );

-- name: UpdateChatAutomationEnabled :one
-- Setting the current state keeps the schedule and updated_at, so the
-- caller still needs update permission and a run in flight is not voided.
-- Turning an automation on clears the cursor of its MCP event subscription,
-- so that polls resume from now and events missed meanwhile do not run.
WITH resumed AS (
    UPDATE chat_automation_mcp_subscriptions s
    SET cursor = NULL
    FROM chat_automations a
    WHERE s.automation_id = @id::uuid AND a.id = s.automation_id AND NOT a.enabled AND @enabled::boolean
)
UPDATE chat_automations
SET
    enabled = @enabled::boolean,
    schedule_next_run_at = CASE WHEN enabled = @enabled::boolean
        THEN schedule_next_run_at
        ELSE sqlc.narg('schedule_next_run_at')::timestamptz END,
    updated_at = CASE WHEN enabled = @enabled::boolean
        THEN updated_at
        ELSE @updated_at::timestamptz END
WHERE id = @id::uuid
RETURNING *;
