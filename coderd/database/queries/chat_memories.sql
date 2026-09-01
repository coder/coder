-- All queries in this file key on root_chat_id and expect the caller to
-- canonicalize subagent chat IDs to their root chat first; a subagent chat
-- ID simply matches no rows on reads (writes are rejected by the
-- chat_memory_root_chat_required trigger).
--
-- There is deliberately no upsert query: the memory tool's create operation
-- must not clobber an existing document, so callers use Insert (fails on
-- duplicate path) and Update (fails on missing path) explicitly.
--
-- Memory inserts require READ COMMITTED; the insert trigger rejects
-- REPEATABLE READ and SERIALIZABLE (READ UNCOMMITTED is accepted because
-- PostgreSQL executes it with READ COMMITTED semantics), so callers must
-- not wrap them in database.ReadModifyUpdate.
--
-- The insert trigger also locks the parent chats row, so a transaction that
-- holds a lock on any chat-owned child row and then inserts a memory for
-- the same root chat inverts the lock order against the retention purge
-- cascade and deadlocks (40P01, which coderd does not retry). Take the
-- chats row lock first: GetChatByIDForUpdate, or ChatMachine.Update, which
-- opens with LockChatAndBumpSnapshotVersion (LockChatByID is system-scoped
-- and not callable as the user). Or do not mix the insert with prior
-- child-row writes in one transaction.
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

-- Lists order by path with an explicit collation so results are stable
-- across deployments with different database default collations.
-- name: ListChatMemoriesByRootChatID :many
SELECT
    id, root_chat_id, path, created_at, updated_at
FROM chat_memories
WHERE root_chat_id = @root_chat_id
ORDER BY path COLLATE "C" ASC;

-- An empty path_prefix matches every row, because starts_with returns true
-- for an empty prefix. The prefix delete below instead treats an empty
-- prefix as "no rows", so a bad argument previews everything but deletes
-- nothing.
-- name: ListChatMemoriesByRootChatIDAndPathPrefix :many
SELECT
    id, root_chat_id, path, created_at, updated_at,
    -- Read 4096 characters for YAML frontmatter indexing. The table separately
    -- caps stored content at 64 KiB.
    left(content, 4096)::text AS content_prefix
FROM chat_memories
WHERE root_chat_id = @root_chat_id AND starts_with(path, @path_prefix::text)
ORDER BY path COLLATE "C" ASC;

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
RETURNING id, root_chat_id, path, created_at, updated_at;
