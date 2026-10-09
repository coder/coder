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

-- name: GetChatProjectsByOwnerID :many
SELECT *
FROM chat_projects
WHERE owner_id = @owner_id::uuid AND NOT deleted
ORDER BY lower(name), id;

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

-- name: DeleteChatProjectByID :exec
DELETE FROM chat_projects
WHERE id = @id::uuid;

-- name: UpdateChatProjectDeletedByID :exec
-- Irreversible: dbpurge deletes the project's chat families on its next
-- tick, regardless of chat retention.
UPDATE chat_projects
SET deleted = true, updated_at = now()
WHERE id = @id::uuid AND NOT deleted;

-- name: CountChatProjectsByOwnerID :one
SELECT COUNT(*)::bigint
FROM chat_projects
WHERE owner_id = @owner_id::uuid AND NOT deleted;

-- name: GetChatProjectByIDForShare :one
-- Root chat creation holds this until commit, so a concurrent project
-- delete either waits for the new chat or is seen by the creation.
SELECT *
FROM chat_projects
WHERE id = @id::uuid AND NOT deleted
FOR SHARE;

-- name: GetChatProjectChatFamilies :many
SELECT *
FROM chats_expanded
WHERE id IN (
    SELECT chats.id FROM chats WHERE chats.project_id = @project_id::uuid
    UNION ALL
    SELECT child.id
    FROM chats child
    JOIN chats root ON root.id = child.root_chat_id
    WHERE root.project_id = @project_id::uuid
)
ORDER BY id;

-- name: LockChatProjectRootChats :exec
-- Waits for sub-chat creations under the roots, which hold the root
-- FOR SHARE, so a later statement sees every committed child.
SELECT id
FROM chats
WHERE project_id = @project_id::uuid AND parent_chat_id IS NULL
ORDER BY id
FOR UPDATE;

-- name: DeleteChatQueuedMessagesByChatIDs :exec
DELETE FROM chat_queued_messages WHERE chat_id = ANY(@chat_ids::uuid[]);

-- name: ArchiveChatsOfDeletedChatProject :exec
-- Forces chats into an archived idle state with no lease, so readers that
-- skip archived chats skip them, workers stop at their next renewal and do
-- not reacquire them, and no sub-chat can join their families. Callers
-- clear the queue first, because archived waiting chats have none.
UPDATE chats
SET
    archived = true,
    pin_order = 0,
    status = CASE WHEN status = 'error'::chat_status THEN 'error'::chat_status ELSE 'waiting'::chat_status END,
    worker_id = NULL,
    runner_id = NULL,
    requires_action_deadline_at = NULL,
    compaction_requested_at = NULL,
    retry_state = NULL,
    snapshot_version = snapshot_version + 1,
    updated_at = now()
WHERE id = ANY(@chat_ids::uuid[]);

-- name: IsChatInDeletedProject :one
SELECT EXISTS (
    SELECT 1
    FROM chats c
    JOIN chats root ON root.id = COALESCE(c.root_chat_id, c.parent_chat_id, c.id)
    JOIN chat_projects ON chat_projects.id = root.project_id
    WHERE c.id = @chat_id::uuid AND chat_projects.deleted
)::boolean;

-- name: LockDeletedChatProjectRootChats :many
-- Locking the roots first makes the family delete, a separate statement,
-- see sub-chats that committed while this one waited.
SELECT chats.id
FROM chats
JOIN chat_projects ON chat_projects.id = chats.project_id
WHERE chat_projects.deleted AND chats.parent_chat_id IS NULL
ORDER BY chats.id
LIMIT @limit_count
FOR UPDATE OF chats;

-- name: DeleteChatFamiliesByRootIDs :execrows
DELETE FROM chats
WHERE id IN (
    SELECT unnest(@root_ids::uuid[])
    UNION ALL
    SELECT chats.id FROM chats WHERE chats.root_chat_id = ANY(@root_ids::uuid[])
);

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
