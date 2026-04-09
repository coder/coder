-- name: InsertChatFile :one
INSERT INTO chat_files (owner_id, organization_id, name, mimetype, data)
VALUES (@owner_id::uuid, @organization_id::uuid, @name::text, @mimetype::text, @data::bytea)
RETURNING id, owner_id, organization_id, created_at, name, mimetype;

-- name: GetChatFileByID :one
SELECT * FROM chat_files WHERE id = @id::uuid;

-- name: GetChatFilesByIDs :many
SELECT * FROM chat_files WHERE id = ANY(@ids::uuid[]);

-- name: GetChatFileMetadataByChatID :many
-- GetChatFileMetadataByChatID returns lightweight file metadata for
-- all files linked to a chat. The data column is excluded to avoid
-- loading file content.
SELECT cf.id, cf.owner_id, cf.organization_id, cf.name, cf.mimetype, cf.created_at
FROM chat_files cf
JOIN chat_file_links cfl ON cfl.file_id = cf.id
WHERE cfl.chat_id = @chat_id::uuid
ORDER BY cf.created_at ASC;

-- TODO(cian): Add indexes on chats(archived, updated_at) and
-- chat_files(created_at) for purge query performance.
-- See: https://github.com/coder/internal/issues/1438
-- name: DeleteOldChatFiles :execrows
-- Deletes chat files that are older than the given threshold and are
-- not referenced by any chat that is still active or was archived
-- within the same threshold window. This covers two cases:
-- 1. Orphaned files not linked to any chat.
-- 2. Files whose every referencing chat has been archived for longer
--    than the retention period.
WITH kept_file_ids AS (
    -- NOTE: This uses updated_at as a proxy for archive time
    -- because there is no archived_at column. Correctness
    -- requires that updated_at is never backdated on archived
    -- chats. See ArchiveChatByID.
    SELECT DISTINCT cfl.file_id
    FROM chat_file_links cfl
    JOIN chats c ON c.id = cfl.chat_id
    WHERE c.archived = false
       OR c.updated_at >= @before_time::timestamptz
),
deletable AS (
    SELECT cf.id
    FROM chat_files cf
    LEFT JOIN kept_file_ids k ON cf.id = k.file_id
    WHERE cf.created_at < @before_time::timestamptz
      AND k.file_id IS NULL
    ORDER BY cf.created_at ASC
    LIMIT @limit_count
)
DELETE FROM chat_files
USING deletable
WHERE chat_files.id = deletable.id;
