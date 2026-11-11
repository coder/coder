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

-- name: GetChatProjectByIDForShare :one
-- Root chat creation in a project, moves into a project, and chat
-- unarchives hold this until commit, so they serialize with a project
-- archive, which locks the project FOR UPDATE first.
SELECT *
FROM chat_projects
WHERE id = @id::uuid
FOR SHARE;

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

-- name: UpdateChatProjectArchivedAt :one
-- Call only through chatstate.ArchiveProjectChats and
-- chatstate.UnarchiveProjectChats, which archive and unarchive the
-- project's chats in the same transaction.
UPDATE chat_projects
SET
    archived_at = sqlc.narg('archived_at')::timestamptz,
    updated_at = now()
WHERE id = @id::uuid
RETURNING *;

-- name: CountChatProjectArchivePreview :one
-- Counts the unarchived root chats of a project, of every owner, and the
-- ones whose family has a member that archiving the project would stop.
-- Subagent chats have no project_id and belong to their root's project.
WITH roots AS (
    SELECT chats.id
    FROM chats
    WHERE chats.project_id = @project_id::uuid
        AND chats.parent_chat_id IS NULL
        AND NOT chats.archived
)
SELECT
    (SELECT COUNT(*) FROM roots)::bigint AS chat_count,
    (
        SELECT COUNT(*)
        FROM roots
        WHERE EXISTS (
            SELECT 1
            FROM chats member
            WHERE (member.id = roots.id OR member.root_chat_id = roots.id)
                AND member.status IN (
                    'running'::chat_status,
                    'interrupting'::chat_status,
                    'requires_action'::chat_status,
                    'paused'::chat_status
                )
        )
    )::bigint AS active_chat_count;

-- name: LockChatProjectUnarchivedRootChats :exec
-- Sub-chat creation holds its root FOR SHARE, so this waits for it. At
-- READ COMMITTED, the default isolation, a later statement then sees every
-- child committed during the wait.
SELECT id
FROM chats
WHERE project_id = @project_id::uuid AND parent_chat_id IS NULL AND NOT archived
ORDER BY id
FOR UPDATE;

-- name: ArchiveChatsOfChatProject :many
-- Archives every unarchived chat of the project, root chats and their
-- subagent chats, and marks them archived_with_project. This is an
-- execution-state transition outside the chatstate machine: keep the SET
-- list in sync with UpdateChatExecutionState. Queued messages stay; a chat
-- with a queue moves to error with stopped_error unless it already is in
-- error, so it lands in XE1 or XE1P instead of an invalid state. Publishes
-- nothing, so callers publish chat:update per chat.
WITH roots AS (
    SELECT chats.id
    FROM chats
    WHERE chats.project_id = @project_id::uuid
        AND chats.parent_chat_id IS NULL
        AND NOT chats.archived
),
targets AS (
    SELECT
        chats.id,
        EXISTS (
            SELECT 1
            FROM chat_queued_messages
            WHERE chat_queued_messages.chat_id = chats.id
        ) AS has_queue
    FROM chats
    WHERE (chats.id IN (SELECT id FROM roots) OR chats.root_chat_id IN (SELECT id FROM roots))
        AND NOT chats.archived
)
UPDATE chats
SET
    archived = true,
    archived_with_project = true,
    pin_order = 0,
    status = CASE
        WHEN chats.status = 'error'::chat_status OR targets.has_queue THEN 'error'::chat_status
        ELSE 'waiting'::chat_status
    END,
    last_error = CASE
        WHEN chats.status <> 'error'::chat_status AND targets.has_queue THEN @stopped_error::jsonb
        ELSE chats.last_error
    END,
    worker_id = NULL,
    runner_id = NULL,
    requires_action_deadline_at = NULL,
    compaction_requested_at = NULL,
    retry_state = NULL,
    snapshot_version = chats.snapshot_version + 1,
    updated_at = now()
FROM targets
WHERE chats.id = targets.id
RETURNING chats.id;

-- name: UnarchiveChatsOfChatProject :many
-- Unarchives the chats that archiving the project archived, root chats
-- and their subagent chats. Chats archived on their own stay archived.
-- Execution status is untouched: an archived chat is idle or in error.
-- Only families whose root was archived with the project come back, so no
-- family ends up partly archived.
WITH roots AS (
    SELECT chats.id
    FROM chats
    WHERE chats.project_id = @project_id::uuid
        AND chats.parent_chat_id IS NULL
        AND chats.archived
        AND chats.archived_with_project
)
UPDATE chats
SET
    archived = false,
    archived_with_project = false,
    snapshot_version = chats.snapshot_version + 1,
    updated_at = now()
WHERE (chats.id IN (SELECT id FROM roots) OR chats.root_chat_id IN (SELECT id FROM roots))
    AND chats.archived
    AND chats.archived_with_project
RETURNING chats.id;
