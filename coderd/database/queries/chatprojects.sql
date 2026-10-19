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

-- name: GetChatProjectByIDForUpdate :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid
FOR UPDATE;

-- name: GetChatProjectsOwnedOrSharedWithUserID :many
-- Entries match regardless of the actions they grant, so callers must
-- authorize each row.
SELECT chat_projects.*
FROM chat_projects
WHERE chat_projects.owner_id = @user_id::uuid
    OR (
        (
            chat_projects.user_acl ? (@user_id::uuid)::text
            OR chat_projects.group_acl ?| ARRAY(
                SELECT group_members.group_id::text
                FROM group_members
                WHERE group_members.user_id = @user_id::uuid
                UNION ALL
                SELECT organization_members.organization_id::text
                FROM organization_members
                WHERE organization_members.user_id = @user_id::uuid
            )
        )
        AND EXISTS (
            SELECT 1
            FROM organization_members
            WHERE organization_members.user_id = @user_id::uuid
                AND organization_members.organization_id = chat_projects.organization_id
        )
    )
ORDER BY lower(chat_projects.name), chat_projects.id;

-- name: IsChatProjectUsableByUserID :one
SELECT EXISTS (
    SELECT 1
    FROM chat_projects
    WHERE chat_projects.id = @project_id::uuid
        AND (
            chat_projects.owner_id = @user_id::uuid
            OR (
                EXISTS (
                    SELECT 1
                    FROM organization_members
                    WHERE organization_members.user_id = @user_id::uuid
                        AND organization_members.organization_id = chat_projects.organization_id
                )
                AND (
                    chat_projects.user_acl -> (@user_id::uuid)::text -> 'permissions' ?| ARRAY['read', '*']
                    OR chat_projects.group_acl -> chat_projects.organization_id::text -> 'permissions' ?| ARRAY['read', '*']
                    OR EXISTS (
                        SELECT 1
                        FROM group_members
                        WHERE group_members.user_id = @user_id::uuid
                            AND chat_projects.group_acl -> group_members.group_id::text -> 'permissions' ?| ARRAY['read', '*']
                    )
                )
            )
        )
)::boolean;

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
    -- The template columns change together, and only when the request sets
    -- templates, so a concurrent update that omits them cannot restore a
    -- stale snapshot.
    template_ids = CASE WHEN @update_templates::boolean THEN @template_ids::uuid[] ELSE template_ids END,
    default_template_id = CASE WHEN @update_templates::boolean THEN sqlc.narg('default_template_id')::uuid ELSE default_template_id END,
    default_template_version_preset_id = CASE WHEN @update_templates::boolean THEN sqlc.narg('default_template_version_preset_id')::uuid ELSE default_template_version_preset_id END,
    default_parameters = CASE WHEN @update_templates::boolean THEN COALESCE(sqlc.narg('default_parameters')::jsonb, '{}'::jsonb) ELSE default_parameters END,
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
