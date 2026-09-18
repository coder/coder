-- name: InsertChatProject :one
INSERT INTO chat_projects (id, organization_id, created_by, name, description)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @created_by::uuid,
    @name::text,
    @description::text
)
RETURNING *;

-- name: GetChatProjectByID :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid;

-- name: GetChatProjectsByOrganizationID :many
SELECT *
FROM chat_projects
WHERE organization_id = @organization_id::uuid
    -- Authorize Filter clause will be injected below in GetAuthorizedChatProjects
    -- @authorize_filter
ORDER BY lower(name);

-- name: UpdateChatProjectByID :one
UPDATE chat_projects
SET
    name = @name::text,
    description = @description::text,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: DeleteChatProjectByID :exec
DELETE FROM chat_projects
WHERE id = @id::uuid;

-- name: GetChatProjectByIDForUpdate :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid
FOR UPDATE;

-- name: GetChatProjectACLByID :one
SELECT
    user_acl AS users,
    group_acl AS groups
FROM chat_projects
WHERE id = @id::uuid;

-- name: UpdateChatProjectACLByID :exec
UPDATE chat_projects
SET
    user_acl = @user_acl,
    group_acl = @group_acl,
    updated_at = now()
WHERE id = @id::uuid;
