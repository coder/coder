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

-- name: IsChatProjectUsableByUserID :one
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

-- name: UpdateChatProjectDeletedByID :one
-- Irreversible. Waits for root chat creations holding
-- GetChatProjectByIDForShare. Deletes the project's memories,
-- instructions and skills; its chats keep running until
-- chatstate.ArchiveDeletedChatProject archives them.
WITH project AS (
    UPDATE chat_projects
    SET deleted = true, updated_at = now()
    WHERE id = @id::uuid AND NOT deleted
    RETURNING id
),
memories AS (
    DELETE FROM chat_project_memories
    WHERE project_id IN (SELECT id FROM project)
),
instructions AS (
    DELETE FROM chat_project_instructions
    WHERE project_id IN (SELECT id FROM project)
),
skills AS (
    DELETE FROM skills
    WHERE project_id IN (SELECT id FROM project)
)
SELECT id FROM project;

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

-- name: GetChatsByIDs :many
SELECT *
FROM chats_expanded
WHERE id = ANY(@ids::uuid[])
ORDER BY id;

-- name: LockChatProjectRootChats :exec
-- Sub-chat creation holds its root FOR SHARE, so this waits for it. At
-- READ COMMITTED, the default isolation, a later statement then sees every
-- child committed during the wait.
SELECT id
FROM chats
WHERE project_id = @project_id::uuid AND parent_chat_id IS NULL AND NOT archived
ORDER BY id
FOR UPDATE;

-- name: ArchiveChatsOfDeletedChatProject :many
-- Execution-state transition outside the chatstate machine; keep the SET
-- list in sync with UpdateChatExecutionState. Publishes nothing, so callers
-- publish chat:update per chat. Run it before
-- DeleteChatQueuedMessagesOfDeletedChatProject so a send waiting on a chat
-- lock reads the archived state and is refused.
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
DELETE FROM chat_queued_messages
WHERE chat_id = ANY(@chat_ids::uuid[])
    AND chat_id IN (
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
WITH empty AS (
    SELECT chat_projects.id
    FROM chat_projects
    WHERE chat_projects.deleted
        AND NOT EXISTS (SELECT 1 FROM chats WHERE chats.project_id = chat_projects.id)
    LIMIT @limit_count
)
DELETE FROM chat_projects
WHERE id IN (SELECT id FROM empty);
