-- name: InsertUserSkill :one
INSERT INTO skills (id, user_id, name, description, content)
VALUES (@id::uuid, @user_id::uuid, @name::text, @description::text, @content::text)
RETURNING *;

-- name: GetUserSkillByUserIDAndName :one
SELECT *
FROM skills
WHERE user_id = @user_id::uuid AND name = @name;

-- name: ListUserSkillMetadataByUserID :many
SELECT
    id, user_id, name, description, enabled, created_at, updated_at
FROM skills
WHERE user_id = @user_id::uuid
ORDER BY name ASC;

-- name: UpdateUserSkillByUserIDAndName :one
UPDATE skills
SET
    description = COALESCE(sqlc.narg('description')::text, description),
    content     = COALESCE(sqlc.narg('content')::text, content),
    enabled     = COALESCE(sqlc.narg('enabled')::boolean, enabled),
    updated_at  = now()
WHERE user_id = @user_id::uuid AND name = @name
RETURNING *;

-- name: DeleteUserSkillByUserIDAndName :one
DELETE FROM skills
WHERE user_id = @user_id::uuid AND name = @name
RETURNING *;

-- name: InsertOrganizationSkill :one
INSERT INTO skills (id, organization_id, name, description, content, group_acl, user_acl)
VALUES (@id::uuid, @organization_id::uuid, @name::text, @description::text, @content::text, @group_acl, @user_acl)
RETURNING *;

-- name: GetOrganizationSkillByOrganizationIDAndName :one
SELECT *
FROM skills
WHERE organization_id = @organization_id::uuid AND name = @name;

-- name: ListOrganizationSkillMetadataByOrganizationID :many
SELECT
    id, organization_id, name, description, enabled, created_at, updated_at
FROM skills
WHERE organization_id = @organization_id::uuid
    -- Authorize Filter clause will be injected below in GetAuthorizedOrganizationSkillMetadata
    -- @authorize_filter
ORDER BY name ASC;

-- name: UpdateOrganizationSkillByOrganizationIDAndName :one
UPDATE skills
SET
    description = COALESCE(sqlc.narg('description')::text, description),
    content     = COALESCE(sqlc.narg('content')::text, content),
    enabled     = COALESCE(sqlc.narg('enabled')::boolean, enabled),
    updated_at  = now()
WHERE organization_id = @organization_id::uuid AND name = @name
RETURNING *;

-- name: DeleteOrganizationSkillByOrganizationIDAndName :one
DELETE FROM skills
WHERE organization_id = @organization_id::uuid AND name = @name
RETURNING *;

-- name: GetOrganizationSkillByIDForUpdate :one
SELECT *
FROM skills
WHERE id = @id::uuid AND organization_id IS NOT NULL
FOR UPDATE;

-- name: UpdateOrganizationSkillACLByID :one
UPDATE skills
SET
    group_acl  = @group_acl,
    user_acl   = @user_acl,
    updated_at = now()
WHERE id = @id::uuid AND organization_id IS NOT NULL
RETURNING *;
