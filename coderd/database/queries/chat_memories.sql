-- name: InsertChatMemory :one
INSERT INTO chat_memories (id, root_chat_id, path, content)
VALUES (@id::uuid, @root_chat_id::uuid, @path::text, @content::text)
RETURNING *;

-- name: GetChatMemoryByID :one
SELECT *
FROM chat_memories
WHERE id = @id;

-- name: GetChatMemoryByRootChatIDAndPath :one
SELECT *
FROM chat_memories
WHERE root_chat_id = @root_chat_id AND path = @path;

-- name: ListChatMemoriesByRootChatID :many
SELECT
    id, root_chat_id, path, created_at, updated_at
FROM chat_memories
WHERE root_chat_id = @root_chat_id
ORDER BY path ASC;

-- name: ListChatMemoriesByRootChatIDAndPathPrefix :many
SELECT
    id, root_chat_id, path, created_at, updated_at,
    left(content, 4096)::text AS content_prefix
FROM chat_memories
WHERE root_chat_id = @root_chat_id AND starts_with(path, @path_prefix::text)
ORDER BY path ASC;

-- name: UpdateChatMemoryByRootChatIDAndPath :one
UPDATE chat_memories
SET
    content    = @content,
    updated_at = now()
WHERE root_chat_id = @root_chat_id AND path = @path
RETURNING *;

-- name: RenameChatMemoryByRootChatIDAndPath :one
UPDATE chat_memories
SET
    path       = @new_path::text,
    updated_at = now()
WHERE root_chat_id = @root_chat_id AND path = @old_path::text
RETURNING *;

-- name: DeleteChatMemoryByRootChatIDAndPath :one
DELETE FROM chat_memories
WHERE root_chat_id = @root_chat_id AND path = @path
RETURNING *;

-- name: DeleteChatMemoriesByRootChatIDAndPathPrefix :many
DELETE FROM chat_memories
WHERE
    root_chat_id = @root_chat_id
    AND octet_length(@path_prefix::text) > 0
    AND starts_with(path, @path_prefix::text)
RETURNING *;
