-- name: InsertAutomation :one
INSERT INTO automations (
    owner_id,
    organization_id,
    name,
    description,
    webhook_secret,
    cron_schedule,
    filter,
    label_paths,
    instructions,
    model_config_id,
    mcp_server_ids,
    allowed_tools,
    status,
    max_chat_creates_per_hour,
    max_messages_per_hour
) VALUES (
    @owner_id::uuid,
    @organization_id::uuid,
    @name::text,
    @description::text,
    sqlc.narg('webhook_secret')::text,
    sqlc.narg('cron_schedule')::text,
    sqlc.narg('filter')::jsonb,
    sqlc.narg('label_paths')::jsonb,
    @instructions::text,
    sqlc.narg('model_config_id')::uuid,
    COALESCE(@mcp_server_ids::uuid[], '{}'::uuid[]),
    COALESCE(@allowed_tools::text[], '{}'::text[]),
    @status::text,
    @max_chat_creates_per_hour::integer,
    @max_messages_per_hour::integer
) RETURNING *;

-- name: GetAutomationByID :one
SELECT * FROM automations WHERE id = @id::uuid;

-- name: GetAutomations :many
SELECT
    *
FROM
    automations
WHERE
    CASE
        WHEN @owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN automations.owner_id = @owner_id
        ELSE true
    END
    AND CASE
        WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN automations.organization_id = @organization_id
        ELSE true
    END
    -- Authorize Filter clause will be injected below in GetAuthorizedAutomations
    -- @authorize_filter
ORDER BY
    created_at DESC
OFFSET @offset_opt
LIMIT
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: UpdateAutomation :one
UPDATE automations SET
    name = @name::text,
    description = @description::text,
    cron_schedule = sqlc.narg('cron_schedule')::text,
    filter = sqlc.narg('filter')::jsonb,
    label_paths = sqlc.narg('label_paths')::jsonb,
    instructions = @instructions::text,
    model_config_id = sqlc.narg('model_config_id')::uuid,
    mcp_server_ids = @mcp_server_ids::uuid[],
    allowed_tools = @allowed_tools::text[],
    status = @status::text,
    max_chat_creates_per_hour = @max_chat_creates_per_hour::integer,
    max_messages_per_hour = @max_messages_per_hour::integer,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *;

-- name: UpdateAutomationWebhookSecret :one
UPDATE automations SET
    webhook_secret = sqlc.narg('webhook_secret')::text,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteAutomationByID :exec
DELETE FROM automations WHERE id = @id::uuid;
