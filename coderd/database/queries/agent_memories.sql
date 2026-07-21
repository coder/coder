-- name: InsertAgentMemory :one
INSERT INTO agent_memories (id, user_id, path, content)
VALUES (@id::uuid, @user_id::uuid, @path::text, @content::text)
RETURNING *;

-- name: GetAgentMemoryByUserIDAndPath :one
SELECT *
FROM agent_memories
WHERE user_id = @user_id::uuid
  AND path = @path::text;

-- name: GetAgentMemoryByUserIDAndPathForUpdate :one
SELECT *
FROM agent_memories
WHERE user_id = @user_id::uuid
  AND path = @path::text
FOR UPDATE;

-- name: UpdateAgentMemoryContent :one
UPDATE agent_memories
SET content = @content::text,
    updated_at = now()
WHERE user_id = @user_id::uuid
  AND path = @path::text
RETURNING *;

-- name: DeleteAgentMemoryByUserIDAndPath :one
DELETE FROM agent_memories
WHERE user_id = @user_id::uuid
  AND path = @path::text
RETURNING *;

-- name: SearchAgentMemories :many
WITH search_query AS (
    SELECT websearch_to_tsquery('simple'::regconfig, @keywords::text) AS query
)
SELECT
    agent_memories.*,
    ts_rank_cd(agent_memories.search_vector, search_query.query)::real AS rank,
    CAST(ts_headline(
        'simple'::regconfig,
        replace(
            replace(agent_memories.content, '<memory-hit>', '&lt;memory-hit&gt;'),
            '</memory-hit>',
            '&lt;/memory-hit&gt;'
        ),
        search_query.query,
        'StartSel=<memory-hit>, StopSel=</memory-hit>, HighlightAll=true'
    ) AS varchar) AS headline
FROM agent_memories
CROSS JOIN search_query
WHERE agent_memories.user_id = @user_id::uuid
  AND agent_memories.search_vector @@ search_query.query
  AND (
      cardinality(@path_regexes::text[]) = 0
      OR agent_memories.path ~ ANY(@path_regexes::text[])
  )
ORDER BY rank DESC, agent_memories.path ASC
LIMIT 26
OFFSET @offset_value::int;

-- name: ListAgentMemories :many
SELECT
    id,
    user_id,
    path,
    octet_length(content)::bigint AS size_bytes,
    created_at,
    updated_at
FROM agent_memories
WHERE user_id = @user_id::uuid
  AND COALESCE(
      NULLIF(regexp_replace(path, '/[^/]+$', ''), ''),
      '/'
  ) ~ @directory_regex::text
ORDER BY path ASC
LIMIT 26
OFFSET @offset_value::int;
