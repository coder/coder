-- name: InsertAutomation :one
INSERT INTO automations (
    owner_id,
    organization_id,
    name,
    description,
    webhook_secret,
    filter,
    session_labels,
    system_prompt,
    model_config_id,
    workspace_id,
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
    @webhook_secret::text,
    sqlc.narg('filter')::jsonb,
    sqlc.narg('session_labels')::jsonb,
    @system_prompt::text,
    sqlc.narg('model_config_id')::uuid,
    sqlc.narg('workspace_id')::uuid,
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
    filter = sqlc.narg('filter')::jsonb,
    session_labels = sqlc.narg('session_labels')::jsonb,
    system_prompt = @system_prompt::text,
    model_config_id = sqlc.narg('model_config_id')::uuid,
    workspace_id = sqlc.narg('workspace_id')::uuid,
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
    webhook_secret = @webhook_secret::text,
    updated_at = NOW()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteAutomationByID :exec
DELETE FROM automations WHERE id = @id::uuid;
