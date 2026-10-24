-- name: InsertUserSkill :one
INSERT INTO skills (id, user_id, name, description, content, model_invocable)
VALUES (@id::uuid, @user_id::uuid, @name::text, @description::text, @content::text, COALESCE(sqlc.narg('model_invocable')::boolean, true))
RETURNING *;

-- name: GetUserSkillByUserIDAndName :one
SELECT *
FROM skills
WHERE user_id = @user_id::uuid AND name = @name;

-- name: ListUserSkillMetadataByUserID :many
SELECT
    id, user_id, name, description, enabled, action, model_invocable, created_at, updated_at
FROM skills
WHERE user_id = @user_id::uuid
ORDER BY name ASC;

-- name: UpdateUserSkillByUserIDAndName :one
UPDATE skills
SET
    description = COALESCE(sqlc.narg('description')::text, description),
    content     = COALESCE(sqlc.narg('content')::text, content),
    enabled     = COALESCE(sqlc.narg('enabled')::boolean, enabled),
    model_invocable = COALESCE(sqlc.narg('model_invocable')::boolean, model_invocable),
    updated_at  = now()
WHERE user_id = @user_id::uuid AND name = @name
RETURNING *;

-- name: DeleteUserSkillByUserIDAndName :one
DELETE FROM skills
WHERE user_id = @user_id::uuid AND name = @name
RETURNING *;

-- name: InsertOrganizationSkill :one
INSERT INTO skills (id, organization_id, name, description, content, model_invocable, group_acl, user_acl)
VALUES (@id::uuid, @organization_id::uuid, @name::text, @description::text, @content::text, COALESCE(sqlc.narg('model_invocable')::boolean, true), @group_acl, @user_acl)
RETURNING *;

-- name: GetOrganizationSkillByOrganizationIDAndName :one
SELECT *
FROM skills
WHERE organization_id = @organization_id::uuid AND name = @name;

-- name: ListOrganizationSkillMetadataByOrganizationID :many
SELECT
    id, organization_id, name, description, enabled, action, model_invocable, created_at, updated_at
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
    model_invocable = COALESCE(sqlc.narg('model_invocable')::boolean, model_invocable),
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

-- name: InsertProjectSkill :one
INSERT INTO skills (id, project_id, name, description, content, action, model_invocable)
VALUES (@id::uuid, @project_id::uuid, @name::text, @description::text, @content::text, COALESCE(sqlc.narg('action')::boolean, false), COALESCE(sqlc.narg('model_invocable')::boolean, true))
RETURNING *;

-- name: GetProjectSkillByProjectIDAndName :one
SELECT *
FROM skills
WHERE project_id = @project_id::uuid AND name = @name;

-- name: ListProjectSkillMetadataByProjectID :many
SELECT
    id, project_id, name, description, enabled, action, model_invocable, created_at, updated_at
FROM skills
WHERE project_id = @project_id::uuid
ORDER BY name ASC;

-- name: UpdateProjectSkillByProjectIDAndName :one
UPDATE skills
SET
    description = COALESCE(sqlc.narg('description')::text, description),
    content     = COALESCE(sqlc.narg('content')::text, content),
    enabled     = COALESCE(sqlc.narg('enabled')::boolean, enabled),
    model_invocable = COALESCE(sqlc.narg('model_invocable')::boolean, model_invocable),
    action      = COALESCE(sqlc.narg('action')::boolean, action),
    updated_at  = now()
WHERE project_id = @project_id::uuid AND name = @name
RETURNING *;

-- name: DeleteProjectSkillByProjectIDAndName :one
DELETE FROM skills
WHERE project_id = @project_id::uuid AND name = @name
RETURNING *;

-- name: GetProjectSkillByIDForUpdate :one
SELECT *
FROM skills
WHERE id = @id::uuid AND project_id IS NOT NULL
FOR UPDATE;
