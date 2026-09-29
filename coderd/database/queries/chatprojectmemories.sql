-- name: InsertChatProjectMemory :one
INSERT INTO chat_project_memories (
    id,
    project_id,
    organization_id,
    name,
    description,
    body,
    created_by
)
VALUES (
    COALESCE(sqlc.narg('id')::uuid, gen_random_uuid()),
    @project_id::uuid,
    @organization_id::uuid,
    @name::text,
    @description::text,
    @body::text,
    @created_by::uuid
)
RETURNING *;

-- name: GetChatProjectMemoryByID :one
SELECT
    sqlc.embed(chat_project_memories),
    visible_users.username AS created_by_username
FROM chat_project_memories
JOIN visible_users ON visible_users.id = chat_project_memories.created_by
WHERE chat_project_memories.id = @id::uuid;

-- name: GetChatProjectMemoryByName :one
SELECT
    sqlc.embed(chat_project_memories),
    visible_users.username AS created_by_username
FROM chat_project_memories
JOIN visible_users ON visible_users.id = chat_project_memories.created_by
WHERE chat_project_memories.project_id = @project_id::uuid
    AND lower(chat_project_memories.name) = lower(@name::text);

-- name: GetChatProjectMemoriesByProjectID :many
SELECT
    sqlc.embed(chat_project_memories),
    visible_users.username AS created_by_username
FROM chat_project_memories
JOIN visible_users ON visible_users.id = chat_project_memories.created_by
WHERE chat_project_memories.project_id = @project_id::uuid
ORDER BY lower(chat_project_memories.name);

-- name: CountChatProjectMemoriesByProjectID :one
SELECT COUNT(*)::bigint
FROM chat_project_memories
WHERE project_id = @project_id::uuid;

-- name: DeleteChatProjectMemoryByID :exec
DELETE FROM chat_project_memories
WHERE id = @id::uuid;

-- name: DeleteChatProjectMemoryByName :execrows
DELETE FROM chat_project_memories
WHERE project_id = @project_id::uuid
    AND lower(name) = lower(@name::text);
