-- name: GetChatRuntimeConfig :one
SELECT *
FROM chat_runtime_configs
WHERE organization_id = @organization_id::uuid
    AND runtime = @runtime::chat_runtime;

-- name: ListChatRuntimeConfigs :many
SELECT *
FROM chat_runtime_configs
ORDER BY organization_id, runtime;

-- name: UpsertChatRuntimeConfig :one
INSERT INTO chat_runtime_configs (
    organization_id,
    runtime,
    template_id,
    enabled,
    model,
    permission_mode,
    created_at,
    updated_at
) VALUES (
    @organization_id::uuid,
    @runtime::chat_runtime,
    @template_id::uuid,
    @enabled::boolean,
    @model::text,
    @permission_mode::text,
    now(),
    now()
)
ON CONFLICT (organization_id, runtime) DO UPDATE SET
    template_id = EXCLUDED.template_id,
    enabled = EXCLUDED.enabled,
    model = EXCLUDED.model,
    permission_mode = EXCLUDED.permission_mode,
    updated_at = now()
RETURNING *;

-- name: DeleteChatRuntimeConfig :exec
DELETE FROM chat_runtime_configs
WHERE organization_id = @organization_id::uuid
    AND runtime = @runtime::chat_runtime;
