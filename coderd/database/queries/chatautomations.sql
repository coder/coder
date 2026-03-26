-- name: InsertChatAutomation :one
INSERT INTO chat_automations (
    id,
    owner_id,
    organization_id,
    name,
    description,
    instructions,
    model_config_id,
    mcp_server_ids,
    allowed_tools,
    status,
    max_chat_creates_per_hour,
    max_messages_per_hour,
    created_at,
    updated_at
) VALUES (
    @id::uuid,
    @owner_id::uuid,
    @organization_id::uuid,
    @name::text,
    @description::text,
    @instructions::text,
    sqlc.narg('model_config_id')::uuid,
    COALESCE(@mcp_server_ids::uuid[], '{}'::uuid[]),
    COALESCE(@allowed_tools::text[], '{}'::text[]),
    @status::chat_automation_status,
    @max_chat_creates_per_hour::integer,
    @max_messages_per_hour::integer,
    @created_at::timestamptz,
    @updated_at::timestamptz
) RETURNING *;

-- name: GetChatAutomationByID :one
SELECT * FROM chat_automations WHERE id = @id::uuid;

-- name: GetChatAutomations :many
SELECT
    *
FROM
    chat_automations
WHERE
    CASE
        WHEN @owner_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN chat_automations.owner_id = @owner_id
        ELSE true
    END
    AND CASE
        WHEN @organization_id :: uuid != '00000000-0000-0000-0000-000000000000'::uuid THEN chat_automations.organization_id = @organization_id
        ELSE true
    END
    -- Authorize Filter clause will be injected below in GetAuthorizedChatAutomations
    -- @authorize_filter
ORDER BY
    created_at DESC
OFFSET @offset_opt
LIMIT
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: UpdateChatAutomation :one
UPDATE chat_automations SET
    name = @name::text,
    description = @description::text,
    instructions = @instructions::text,
    model_config_id = sqlc.narg('model_config_id')::uuid,
    mcp_server_ids = COALESCE(@mcp_server_ids::uuid[], '{}'::uuid[]),
    allowed_tools = COALESCE(@allowed_tools::text[], '{}'::text[]),
    status = @status::chat_automation_status,
    max_chat_creates_per_hour = @max_chat_creates_per_hour::integer,
    max_messages_per_hour = @max_messages_per_hour::integer,
    updated_at = @updated_at::timestamptz
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatAutomationByID :exec
DELETE FROM chat_automations WHERE id = @id::uuid;
