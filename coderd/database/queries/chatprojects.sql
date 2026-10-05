-- name: InsertChatProject :one
INSERT INTO chat_projects (
    id, organization_id, owner_id, name, description, icon,
    template_ids, default_template_id, default_template_version_preset_id, default_parameters
)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @owner_id::uuid,
    @name::text,
    @description::text,
    @icon::text,
    COALESCE(@template_ids::uuid[], '{}'::uuid[]),
    sqlc.narg('default_template_id')::uuid,
    sqlc.narg('default_template_version_preset_id')::uuid,
    COALESCE(sqlc.narg('default_parameters')::jsonb, '{}'::jsonb)
)
RETURNING *;

-- name: GetChatProjectByID :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid;

-- name: GetChatProjectsByOwnerID :many
SELECT *
FROM chat_projects
WHERE owner_id = @owner_id::uuid
ORDER BY lower(name), id;

-- name: UpdateChatProjectByID :one
UPDATE chat_projects
SET
    name = @name::text,
    description = @description::text,
    icon = @icon::text,
    template_ids = @template_ids::uuid[],
    default_template_id = sqlc.narg('default_template_id')::uuid,
    default_template_version_preset_id = sqlc.narg('default_template_version_preset_id')::uuid,
    default_parameters = @default_parameters::jsonb,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatProjectByID :exec
DELETE FROM chat_projects
WHERE id = @id::uuid;

-- name: CountChatProjectsByOwnerID :one
SELECT COUNT(*)::bigint
FROM chat_projects
WHERE owner_id = @owner_id::uuid;
