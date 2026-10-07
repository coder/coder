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
-- Locks the row so ACL updates read, modify, and write it in one
-- transaction.
SELECT *
FROM chat_projects
WHERE id = @id::uuid
FOR UPDATE;

-- name: GetChatProjectsAccessibleByUserID :many
-- The Everyone group's ID is the organization ID, so the user's
-- organization IDs join their group IDs. As in the RBAC policy, ACL grants
-- count only for members of the project's organization. ACL keys match
-- regardless of the actions they grant; callers authorize each row.
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

-- name: IsChatProjectAccessibleByUserID :one
-- Reports whether the user owns the project or holds a read grant on it
-- directly, through a group, or through the Everyone group. chatd runs
-- memory tools under its own subject, so it checks this for the chat owner
-- to honor revoked shares. Grants count only for organization members.
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

-- name: GetChatProjectChatsForDelete :many
-- Returns a project's root chats and their sub-chats. The rows stay locked
-- until the transaction ends, so no worker acquires one before
-- DeleteChatProjectChats runs.
SELECT *
FROM chats_expanded
WHERE id IN (
    SELECT chats.id
    FROM chats
    WHERE chats.project_id = @project_id::uuid
        OR chats.root_chat_id IN (
            SELECT root.id FROM chats root WHERE root.project_id = @project_id::uuid
        )
    FOR UPDATE
)
ORDER BY id;

-- name: DeleteChatProjectChats :exec
-- Deletes a project's root chats and their sub-chats. Chat-scoped tables
-- cascade.
DELETE FROM chats
WHERE project_id = @project_id::uuid
    OR root_chat_id IN (
        SELECT root.id FROM chats root WHERE root.project_id = @project_id::uuid
    );
