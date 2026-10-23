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

-- name: InsertProjectSkill :one
INSERT INTO skills (id, project_id, name, description, content)
VALUES (@id::uuid, @project_id::uuid, @name::text, @description::text, @content::text)
RETURNING *;

-- name: GetProjectSkillByProjectIDAndName :one
SELECT *
FROM skills
WHERE project_id = @project_id::uuid AND name = @name;

-- name: ListProjectSkillMetadataByProjectID :many
SELECT
    id, project_id, name, description, enabled, created_at, updated_at
FROM skills
WHERE project_id = @project_id::uuid
ORDER BY name ASC;

-- name: UpdateProjectSkillByProjectIDAndName :one
UPDATE skills
SET
    description = COALESCE(sqlc.narg('description')::text, description),
    content     = COALESCE(sqlc.narg('content')::text, content),
    enabled     = COALESCE(sqlc.narg('enabled')::boolean, enabled),
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

-- name: GetOrganizationSkillMemberPreferences :one
-- Returns the organization's member opt-out setting and the member's stored
-- choices. Members without stored choices get the defaults.
SELECT
    organizations.members_can_disable_skills,
    COALESCE(s.all_skills_disabled, false)::boolean AS all_skills_disabled,
    ARRAY(
        SELECT skill_id
        FROM organization_member_disabled_skills
        WHERE organization_id = @organization_id::uuid AND user_id = @user_id::uuid
        ORDER BY skill_id
    )::uuid[] AS disabled_skill_ids
FROM organizations
LEFT JOIN organization_member_skill_settings s
    ON s.organization_id = organizations.id AND s.user_id = @user_id::uuid
WHERE organizations.id = @organization_id::uuid;

-- name: UpsertOrganizationMemberSkillSettings :exec
INSERT INTO organization_member_skill_settings (organization_id, user_id, all_skills_disabled)
VALUES (@organization_id::uuid, @user_id::uuid, @all_skills_disabled::boolean)
ON CONFLICT (organization_id, user_id) DO UPDATE
SET all_skills_disabled = EXCLUDED.all_skills_disabled;

-- name: InsertOrganizationMemberDisabledSkill :exec
-- Stores nothing unless the skill belongs to the organization.
INSERT INTO organization_member_disabled_skills (organization_id, user_id, skill_id)
SELECT @organization_id::uuid, @user_id::uuid, skills.id
FROM skills
WHERE skills.id = @skill_id::uuid AND skills.organization_id = @organization_id::uuid
ON CONFLICT DO NOTHING;

-- name: DeleteOrganizationMemberDisabledSkill :exec
DELETE FROM organization_member_disabled_skills
WHERE organization_id = @organization_id::uuid
    AND user_id = @user_id::uuid
    AND skill_id = @skill_id::uuid;
