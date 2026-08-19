-- name: InsertChatFile :one
INSERT INTO chat_files (owner_id, organization_id, name, mimetype, data)
VALUES (@owner_id::uuid, @organization_id::uuid, @name::text, @mimetype::text, @data::bytea)
RETURNING id, owner_id, organization_id, created_at, name, mimetype;

-- name: GetChatFileByID :one
SELECT * FROM chat_files WHERE id = @id::uuid;

-- name: GetChatFilesByIDs :many
SELECT * FROM chat_files WHERE id = ANY(@ids::uuid[]);

-- name: GetChatFileDataPrefixesByIDs :many
-- GetChatFileDataPrefixesByIDs returns a bounded prefix of each
-- file's content, keeping full blobs out of server memory. Owner and
-- organization columns support row-level authorization.
SELECT id, owner_id, organization_id, substr(data, 1, @prefix_bytes::int) AS data_prefix
FROM chat_files
WHERE id = ANY(@ids::uuid[]);

-- name: GetChatFileMetadataByChatID :many
-- GetChatFileMetadataByChatID returns lightweight file metadata for
-- all files linked to a chat. The data column is excluded to avoid
-- loading file content.
SELECT cf.id, cf.owner_id, cf.organization_id, cf.name, cf.mimetype, cf.created_at,
	octet_length(cf.data)::bigint AS size_bytes
FROM chat_files cf
JOIN chat_file_links cfl ON cfl.file_id = cf.id
WHERE cfl.chat_id = @chat_id::uuid
ORDER BY cf.created_at ASC;

-- name: GetOldUnlinkedChatFileIDs :many
-- Locks candidate rows against foreign-key inserts for the transaction.
SELECT cf.id
FROM chat_files cf
WHERE cf.created_at < @before_time::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM chat_file_links cfl WHERE cfl.file_id = cf.id
  )
ORDER BY cf.created_at ASC
LIMIT @limit_count
FOR UPDATE OF cf SKIP LOCKED;

-- name: DeleteUnlinkedChatFilesByIDs :execrows
DELETE FROM chat_files cf
WHERE cf.id = ANY(@ids::uuid[])
  AND cf.created_at < @before_time::timestamptz
  AND NOT EXISTS (
    SELECT 1 FROM chat_file_links cfl WHERE cfl.file_id = cf.id
  );
