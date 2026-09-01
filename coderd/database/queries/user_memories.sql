-- There is deliberately no upsert query: the memory tool's create operation
-- must not clobber an existing document, so callers use Insert (fails on
-- duplicate path) and Update (fails on missing path) explicitly.
--
-- Memory inserts require READ COMMITTED; the insert trigger rejects
-- REPEATABLE READ and SERIALIZABLE (READ UNCOMMITTED is accepted because
-- PostgreSQL executes it with READ COMMITTED semantics), so callers must
-- not wrap them in database.ReadModifyUpdate.
--
-- The insert trigger also locks the parent users row, so a transaction that
-- holds a lock on any row that delete_deleted_user_resources deletes
-- (api_keys, user_links, user_secrets, user_skills, user_ai_provider_keys,
-- organization_members, group_members, user_ai_budget_overrides, or a
-- user_memories row) and then inserts a memory
-- for the same user inverts the lock order against that cleanup and
-- deadlocks with a concurrent soft-delete (40P01, which coderd does not
-- retry). Call AcquireUserSoftDeleteGuardLock first (dbauthz authorizes it
-- as a system primitive, not an owner capability: wrap only that call in
-- dbauthz.AsSystemRestricted), or do not mix the insert with prior
-- child-row writes in one transaction.
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

-- Lists order by path with an explicit collation so results are stable
-- across deployments with different database default collations.
-- name: ListUserMemoriesByUserID :many
SELECT
    id, user_id, path, created_at, updated_at
FROM user_memories
WHERE user_id = @user_id
ORDER BY path COLLATE "C" ASC;

-- An empty path_prefix matches every row, because starts_with returns true
-- for an empty prefix. The prefix delete below instead treats an empty
-- prefix as "no rows", so a bad argument previews everything but deletes
-- nothing.
-- name: ListUserMemoriesByUserIDAndPathPrefix :many
SELECT
    id, user_id, path, created_at, updated_at,
    -- Read 4096 characters for YAML frontmatter indexing. The table separately
    -- caps stored content at 64 KiB.
    left(content, 4096)::text AS content_prefix
FROM user_memories
WHERE user_id = @user_id AND starts_with(path, @path_prefix::text)
ORDER BY path COLLATE "C" ASC;

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
WHERE
    user_id = @user_id
    AND octet_length(@path_prefix::text) > 0
    AND starts_with(path, @path_prefix::text)
RETURNING id, user_id, path, created_at, updated_at;
