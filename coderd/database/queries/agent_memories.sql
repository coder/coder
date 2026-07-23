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

-- name: GetAgentMemoryByUserIDAndID :one
SELECT *
FROM agent_memories
WHERE user_id = @user_id::uuid
  AND id = @id::uuid;

-- name: GetAgentMemoryByUserIDAndIDForUpdate :one
SELECT *
FROM agent_memories
WHERE user_id = @user_id::uuid
  AND id = @id::uuid
FOR UPDATE;

-- name: GetDefaultAgentMemoryByUserID :one
SELECT *
FROM agent_memories
WHERE user_id = @user_id::uuid
ORDER BY (path = '/memory.md') DESC, path ASC
LIMIT 1;

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

-- name: DeleteAgentMemoryByUserIDAndID :one
DELETE FROM agent_memories
WHERE user_id = @user_id::uuid
  AND id = @id::uuid
RETURNING *;

-- name: ListAgentMemoryChildren :many
WITH parameters AS (
    SELECT
        @directory::text AS directory,
        CASE
            WHEN @directory::text = '/' THEN '/'
            ELSE @directory::text || '/'
        END AS prefix
), descendants AS (
    SELECT
        agent_memories.*,
        substring(agent_memories.path FROM char_length(parameters.prefix) + 1) AS relative_path
    FROM agent_memories
    CROSS JOIN parameters
    WHERE agent_memories.user_id = @user_id::uuid
      AND left(agent_memories.path, char_length(parameters.prefix)) = parameters.prefix
), entries AS (
    SELECT DISTINCT
        'directory'::text AS kind,
        NULL::uuid AS id,
        (CASE
            WHEN parameters.directory = '/' THEN '/' || split_part(descendants.relative_path, '/', 1)
            ELSE parameters.directory || '/' || split_part(descendants.relative_path, '/', 1)
        END)::text AS path,
        NULL::bigint AS size_bytes,
        NULL::timestamptz AS created_at,
        NULL::timestamptz AS updated_at
    FROM descendants
    CROSS JOIN parameters
    WHERE strpos(descendants.relative_path, '/') > 0

    UNION ALL

    SELECT
        'memory'::text AS kind,
        descendants.id,
        descendants.path,
        octet_length(descendants.content)::bigint AS size_bytes,
        descendants.created_at,
        descendants.updated_at
    FROM descendants
    WHERE strpos(descendants.relative_path, '/') = 0
)
SELECT kind, id, path, size_bytes, created_at, updated_at
FROM entries
ORDER BY (kind = 'memory') ASC, path ASC
LIMIT 26
OFFSET @offset_value::int;

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
