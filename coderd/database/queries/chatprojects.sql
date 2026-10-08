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
WHERE id = @id::uuid AND NOT deleted;

-- name: GetChatProjectByIDForUpdate :one
SELECT *
FROM chat_projects
WHERE id = @id::uuid AND NOT deleted
FOR UPDATE;

-- name: GetChatProjectsOwnedOrSharedWithUserID :many
-- Entries match regardless of the actions they grant, so callers must
-- authorize each row.
SELECT chat_projects.*
FROM chat_projects
WHERE NOT chat_projects.deleted AND (chat_projects.owner_id = @user_id::uuid
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
    ))
ORDER BY lower(chat_projects.name), chat_projects.id;

-- name: IsChatProjectAccessibleByUserID :one
SELECT EXISTS (
    SELECT 1
    FROM chat_projects
    WHERE chat_projects.id = @project_id::uuid
        AND NOT chat_projects.deleted
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
WHERE id = @id::uuid AND NOT deleted;

-- name: UpdateChatProjectByID :one
UPDATE chat_projects
SET
    name = @name::text,
    description = @description::text,
    icon = @icon::text,
    updated_at = now()
WHERE id = @id::uuid AND NOT deleted
RETURNING *;

-- name: MarkChatProjectDeleted :exec
UPDATE chat_projects
SET deleted = true, updated_at = now()
WHERE id = @id::uuid;

-- name: CountChatProjectsByOwnerID :one
SELECT COUNT(*)::bigint
FROM chat_projects
WHERE owner_id = @owner_id::uuid AND NOT deleted;

-- name: GetChatProjectChatFamilies :many
SELECT *
FROM chats_expanded
WHERE project_id = @project_id::uuid
    OR root_chat_id IN (SELECT id FROM chats WHERE chats.project_id = @project_id::uuid)
ORDER BY id;

-- name: IsChatInDeletedProject :one
SELECT EXISTS (
    SELECT 1
    FROM chats c
    JOIN chats root ON root.id = COALESCE(c.root_chat_id, c.parent_chat_id, c.id)
    JOIN chat_projects ON chat_projects.id = root.project_id
    WHERE c.id = @chat_id::uuid AND chat_projects.deleted
)::boolean;

-- name: DeleteChatFamiliesOfDeletedProjects :execrows
WITH roots AS (
    SELECT chats.id
    FROM chats
    JOIN chat_projects ON chat_projects.id = chats.project_id
    WHERE chat_projects.deleted AND chats.parent_chat_id IS NULL
    LIMIT @limit_count
)
DELETE FROM chats
WHERE id IN (SELECT id FROM roots)
    OR root_chat_id IN (SELECT id FROM roots);

-- name: DeleteEmptyDeletedChatProjects :execrows
WITH empty AS (
    SELECT chat_projects.id
    FROM chat_projects
    WHERE chat_projects.deleted
        AND NOT EXISTS (SELECT 1 FROM chats WHERE chats.project_id = chat_projects.id)
    LIMIT @limit_count
)
DELETE FROM chat_projects
WHERE id IN (SELECT id FROM empty);
