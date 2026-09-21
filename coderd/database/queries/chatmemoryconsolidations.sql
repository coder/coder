-- name: InsertChatMemoryConsolidation :one
INSERT INTO chat_memory_consolidations (
    organization_id,
    project_id,
    status,
    model,
    memories_before
)
VALUES (
    @organization_id::uuid,
    @project_id::uuid,
    'running',
    @model::text,
    @memories_before::int
)
RETURNING *;

-- name: FinishChatMemoryConsolidation :one
UPDATE chat_memory_consolidations
SET
    status = @status::chat_memory_consolidation_status,
    finished_at = now(),
    memories_after = @memories_after::int,
    mutations = @mutations::jsonb,
    error = @error::text,
    next_window_start = @next_window_start::int
WHERE id = @id::uuid
RETURNING *;

-- name: GetLatestChatMemoryConsolidationByProject :one
SELECT *
FROM chat_memory_consolidations
WHERE project_id = @project_id::uuid
ORDER BY started_at DESC
LIMIT 1;

-- name: GetChatMemoryConsolidationsByProject :many
SELECT *
FROM chat_memory_consolidations
WHERE project_id = @project_id::uuid
ORDER BY started_at DESC
LIMIT @limit_count::int;

-- name: PruneChatMemoryConsolidationsByProject :exec
DELETE FROM chat_memory_consolidations
WHERE id IN (
    SELECT id
    FROM chat_memory_consolidations
    WHERE project_id = @project_id::uuid
    ORDER BY started_at DESC
    OFFSET @keep_count::int
);
