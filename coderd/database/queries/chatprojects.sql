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
-- Locks FOR NO KEY UPDATE so chat inserts, whose project_id foreign key
-- check takes FOR KEY SHARE, do not wait for it.
SELECT *
FROM chat_projects
WHERE id = @id::uuid AND NOT deleted
FOR NO KEY UPDATE;

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

-- name: UpdateChatProjectDeletedByID :exec
-- Irreversible. Its row lock waits for root chat creations holding
-- GetChatProjectByIDForShare, and later ones find no project. The project's
-- chats keep running until a cleanup transaction, at READ COMMITTED, runs
-- LockChatProjectRootChats, ArchiveChatsOfDeletedChatProject, and
-- DeleteChatQueuedMessagesOfDeletedChatProject in that order.
-- GetDeletedChatProjectIDsWithUnarchivedChats finds projects that still
-- need it.
UPDATE chat_projects
SET deleted = true, updated_at = now()
WHERE id = @id::uuid AND NOT deleted;

-- name: CountChatProjectsByOwnerID :one
SELECT COUNT(*)::bigint
FROM chat_projects
WHERE owner_id = @owner_id::uuid AND NOT deleted;

-- name: GetChatProjectByIDForShare :one
-- Root chat creation in a project must hold this until its insert commits,
-- so a concurrent project delete either waits and then archives the new
-- chat in its cleanup, or makes this return no rows.
SELECT *
FROM chat_projects
WHERE id = @id::uuid AND NOT deleted
FOR SHARE;

-- name: GetChatsByProjectID :many
SELECT *
FROM chats_expanded
WHERE project_id = @project_id::uuid
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

-- name: ArchiveChatsOfDeletedChatProject :many
-- Execution-state transition outside the chatstate machine: moves every
-- unarchived chat of a deleted project to archived waiting, or archived
-- error from error, and returns their IDs; once the queue is cleared these
-- are StateXW and StateXE0.
-- It publishes nothing, so callers publish chat:update per chat. Clearing
-- the runner makes heartbeat renewal stop matching, so workers stop;
-- archived roots refuse new sub-chats. Run it before clearing the queue, so
-- a send waiting on a chat lock reads the archived state and is refused.
-- Keep the SET list in sync with UpdateChatExecutionState.
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
FROM chat_projects
WHERE chats.project_id = @project_id::uuid
    AND NOT chats.archived
    AND chat_projects.id = chats.project_id
    AND chat_projects.deleted
RETURNING chats.id;

-- name: DeleteChatQueuedMessagesOfDeletedChatProject :exec
-- Lives here rather than with the other queue queries because it is a step
-- of the project delete cleanup; see UpdateChatProjectDeletedByID.
DELETE FROM chat_queued_messages
WHERE chat_id IN (
    SELECT chats.id
    FROM chats
    JOIN chat_projects ON chat_projects.id = chats.project_id
    WHERE chat_projects.id = @project_id::uuid AND chat_projects.deleted
);

-- name: GetDeletedChatProjectIDsWithUnarchivedChats :many
SELECT chat_projects.id
FROM chat_projects
WHERE chat_projects.deleted
    AND EXISTS (
        SELECT 1
        FROM chats
        WHERE chats.project_id = chat_projects.id AND NOT chats.archived
    )
ORDER BY chat_projects.updated_at, chat_projects.id
LIMIT @limit_count::int;

-- name: DeleteEmptyDeletedChatProjects :execrows
-- Memories cascade with the row, so they follow chat retention too; nothing
-- can read them after the tombstone.
WITH empty AS (
    SELECT chat_projects.id
    FROM chat_projects
    WHERE chat_projects.deleted
        AND NOT EXISTS (SELECT 1 FROM chats WHERE chats.project_id = chat_projects.id)
    LIMIT @limit_count
)
DELETE FROM chat_projects
WHERE id IN (SELECT id FROM empty);
