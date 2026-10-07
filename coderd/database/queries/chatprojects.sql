-- name: InsertChatProject :one
INSERT INTO chat_projects (id, organization_id, owner_id, name, description, icon)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @organization_id::uuid,
    @owner_id::uuid,
    @name::text,
    @description::text,
    @icon::text
)
RETURNING *;

-- name: GetChatProjectByID :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid;

-- name: GetChatProjectByIDForUpdate :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid
FOR UPDATE;

-- name: GetChatProjectsByOwnerID :many
SELECT *
FROM chat_projects
WHERE owner_id = @owner_id::uuid
ORDER BY lower(name), id;

-- name: GetChatProjectsAccessibleByUserID :many
-- Returns the projects a user owns or that are shared with them directly,
-- through a group, or through the organization's Everyone group, whose ID
-- is the organization ID. ACL grants only apply to organization members,
-- matching the RBAC policy; callers still authorize each row.
SELECT chat_projects.*
FROM chat_projects
WHERE chat_projects.owner_id = @user_id::uuid
    OR (
        EXISTS (
            SELECT 1
            FROM organization_members
            WHERE organization_members.user_id = @user_id::uuid
                AND organization_members.organization_id = chat_projects.organization_id
        )
        AND (
            chat_projects.user_acl ? (@user_id::uuid)::text
            OR chat_projects.group_acl ? chat_projects.organization_id::text
            OR chat_projects.group_acl ?| ARRAY(
                SELECT group_members.group_id::text
                FROM group_members
                WHERE group_members.user_id = @user_id::uuid
            )
        )
    )
ORDER BY lower(chat_projects.name), chat_projects.id;

-- name: UpdateChatProjectACLByID :exec
UPDATE chat_projects
SET
    user_acl = @user_acl,
    group_acl = @group_acl
WHERE id = @id::uuid;

-- name: UpdateChatProjectByID :one
UPDATE chat_projects
SET
    name = @name::text,
    description = @description::text,
    icon = @icon::text,
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
