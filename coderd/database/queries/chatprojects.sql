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
-- Irreversible. Only chatd.DeleteChatProjectWithoutEvents may call this,
-- in one READ COMMITTED transaction, in this order:
--   1. GetChatProjectByIDForUpdate, which waits for root chat creations
--      holding GetChatProjectByIDForShare;
--   2. UpdateChatProjectDeletedByID;
--   3. LockChatProjectRootChats;
--   4. ArchiveChatsOfDeletedChatProject;
--   5. DeleteChatQueuedMessagesOfDeletedChatProject.
-- Calling it alone leaves the project's chats running and acquirable.
-- The archived chats are then removed by chat retention like any other
-- archived chat, and dbpurge deletes the project row once none are left.
UPDATE chat_projects
SET deleted = true, updated_at = now()
WHERE id = @id::uuid AND NOT deleted;

-- name: CountChatProjectsByOwnerID :one
SELECT COUNT(*)::bigint
FROM chat_projects
WHERE owner_id = @owner_id::uuid AND NOT deleted;

-- name: GetChatProjectByIDForShare :one
-- Root chat creation in a project must hold this until its insert commits,
-- so a concurrent project delete either archives the new chat or makes
-- this return no rows.
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
-- Sub-chat creation holds its root FOR SHARE, so this waits for it. At
-- READ COMMITTED, the default isolation, a later statement then sees every
-- child committed during the wait.
SELECT id
FROM chats
WHERE project_id = @project_id::uuid AND parent_chat_id IS NULL
ORDER BY id
FOR UPDATE;

-- name: ArchiveChatsOfDeletedChatProject :exec
-- Execution-state transition outside chatstate: moves every family member
-- of a deleted project from any state to StateXW, or StateXE0 from error,
-- and publishes nothing. Clearing the runner makes heartbeat renewal stop
-- matching, so workers stop; archived roots refuse new sub-chats. Run it
-- before clearing the queue, so a send waiting on a chat lock reads the
-- archived state and is refused. Keep the SET list in sync with
-- UpdateChatExecutionState.
WITH family AS (
    SELECT chats.id
    FROM chats
    JOIN chat_projects ON chat_projects.id = chats.project_id
    WHERE chat_projects.id = @project_id::uuid AND chat_projects.deleted
    UNION ALL
    SELECT child.id
    FROM chats child
    JOIN chats root ON root.id = child.root_chat_id
    JOIN chat_projects ON chat_projects.id = root.project_id
    WHERE chat_projects.id = @project_id::uuid AND chat_projects.deleted
)
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
WHERE id IN (SELECT id FROM family);

-- name: DeleteChatQueuedMessagesOfDeletedChatProject :exec
-- Lives here rather than with the other queue queries because it is a step
-- of the project delete; see UpdateChatProjectDeletedByID.
DELETE FROM chat_queued_messages
WHERE chat_id IN (
    SELECT chats.id
    FROM chats
    JOIN chat_projects ON chat_projects.id = chats.project_id
    WHERE chat_projects.id = @project_id::uuid AND chat_projects.deleted
    UNION ALL
    SELECT child.id
    FROM chats child
    JOIN chats root ON root.id = child.root_chat_id
    JOIN chat_projects ON chat_projects.id = root.project_id
    WHERE chat_projects.id = @project_id::uuid AND chat_projects.deleted
);

-- name: IsChatInDeletedProject :one
SELECT EXISTS (
    SELECT 1
    FROM chats c
    JOIN chats root ON root.id = COALESCE(c.root_chat_id, c.parent_chat_id, c.id)
    JOIN chat_projects ON chat_projects.id = root.project_id
    WHERE c.id = @chat_id::uuid AND chat_projects.deleted
)::boolean;

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
