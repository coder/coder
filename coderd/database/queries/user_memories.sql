-- name: InsertUserMemory :one
INSERT INTO user_memories (id, user_id, path, content)
VALUES (@id::uuid, @user_id::uuid, @path::text, @content::text)
RETURNING *;

-- name: GetUserMemoryByID :one
SELECT *
FROM user_memories
WHERE id = @id;

-- name: GetUserMemoryByUserIDAndPath :one
SELECT *
FROM user_memories
WHERE user_id = @user_id AND path = @path;

-- name: ListUserMemoriesByUserID :many
SELECT
    id, user_id, path, created_at, updated_at
FROM user_memories
WHERE user_id = @user_id
ORDER BY path ASC;

-- name: ListUserMemoriesByUserIDAndPathPrefix :many
SELECT
    id, user_id, path, created_at, updated_at,
    left(content, 4096)::text AS content_prefix
FROM user_memories
WHERE user_id = @user_id AND starts_with(path, @path_prefix::text)
ORDER BY path ASC;

-- name: UpdateUserMemoryByUserIDAndPath :one
UPDATE user_memories
SET
    content    = @content,
    updated_at = now()
WHERE user_id = @user_id AND path = @path
RETURNING *;

-- name: RenameUserMemoryByUserIDAndPath :one
UPDATE user_memories
SET
    path       = @new_path::text,
    updated_at = now()
WHERE user_id = @user_id AND path = @old_path::text
RETURNING *;

-- name: DeleteUserMemoryByUserIDAndPath :one
DELETE FROM user_memories
WHERE user_id = @user_id AND path = @path
RETURNING *;

-- name: DeleteUserMemoriesByUserIDAndPathPrefix :many
DELETE FROM user_memories
WHERE user_id = @user_id AND starts_with(path, @path_prefix::text)
RETURNING *;
