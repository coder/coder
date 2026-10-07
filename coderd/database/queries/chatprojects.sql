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

-- name: GetChatProjectsByOwnerID :many
SELECT *
FROM chat_projects
WHERE owner_id = @owner_id::uuid
ORDER BY lower(name), id;

-- name: GetChatProjectsOwnedOrSharedWithUserID :many
-- Lists projects the user owns or holds any ACL entry on, directly, through
-- a group, or through the Everyone group, whose ID is the organization ID.
-- As in the RBAC policy, ACL entries count only for members of the
-- project's organization. Entries match regardless of the actions they
-- grant, so callers must authorize each row.
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
-- Reports whether the user owns the project or holds an ACL entry granting
-- read or '*' on it, directly, through a group, or through the Everyone
-- group. Entries count only for organization members. It checks a user
-- other than the caller, for code that runs under a system subject.
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

-- name: LockChatProjectChatsForDelete :many
-- Locks up to limit_count of a project's root chats with their sub-chats,
-- returning the locked rows' current worker fields so callers can tell
-- whether a worker holds one. Deleting a project in batches keeps each
-- transaction short. Callers must first lock the project row with
-- GetChatProjectByIDForUpdate in the same transaction, which blocks new
-- chats from joining the project. Roots lock before sub-chats, the order
-- chat archiving uses, to avoid deadlocks.
WITH roots AS (
    SELECT chats.id
    FROM chats
    WHERE chats.project_id = @project_id::uuid
    ORDER BY chats.id
    LIMIT @limit_count::int
)
SELECT chats.id, chats.worker_id, chats.runner_id
FROM chats
-- ANY(ARRAY(...)) lets each branch use its index; IN (subquery) under OR
-- scans the table.
WHERE chats.id = ANY(ARRAY(SELECT id FROM roots))
    OR chats.root_chat_id = ANY(ARRAY(SELECT id FROM roots))
ORDER BY (chats.root_chat_id IS NULL) DESC, chats.id
FOR UPDATE;

-- name: GetChatsByIDs :many
SELECT *
FROM chats_expanded
WHERE id = ANY(@ids::uuid[])
ORDER BY id;

-- name: DeleteChatsByIDs :exec
-- Chat-scoped tables cascade.
DELETE FROM chats
WHERE id = ANY(@ids::uuid[]);
