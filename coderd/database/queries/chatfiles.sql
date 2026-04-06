-- name: InsertChatFile :one
INSERT INTO chat_files (owner_id, organization_id, name, mimetype, data)
VALUES (@owner_id::uuid, @organization_id::uuid, @name::text, @mimetype::text, @data::bytea)
RETURNING id, owner_id, organization_id, created_at, name, mimetype;

-- name: GetChatFileByID :one
SELECT * FROM chat_files WHERE id = @id::uuid;

-- name: GetChatFilesByIDs :many
SELECT * FROM chat_files WHERE id = ANY(@ids::uuid[]);

-- name: DeleteOrphanedChatFiles :execrows
-- Deletes chat_files rows older than the given threshold that are
-- not referenced by any non-deleted chat message. File references
-- live inside the JSONB content array of chat_messages as
-- {"file_id": "<uuid>"} entries in file-type parts.
DELETE FROM chat_files
WHERE created_at < @before::timestamptz
AND NOT EXISTS (
    SELECT 1
    FROM chat_messages cm,
         jsonb_array_elements(cm.content) AS elem
    WHERE (elem ->> 'file_id')::uuid = chat_files.id
    AND cm.deleted = false
);
