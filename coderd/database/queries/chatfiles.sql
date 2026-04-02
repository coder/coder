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
