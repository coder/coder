-- name: InsertChatFile :one
INSERT INTO chat_files (owner_id, organization_id, name, mimetype, data)
VALUES (@owner_id::uuid, @organization_id::uuid, @name::text, @mimetype::text, @data::bytea)
RETURNING id, owner_id, organization_id, created_at, name, mimetype;

-- name: GetChatFileByID :one
SELECT * FROM chat_files WHERE id = @id::uuid;

-- name: GetChatFilesByIDs :many
SELECT * FROM chat_files WHERE id = ANY(@ids::uuid[]);
