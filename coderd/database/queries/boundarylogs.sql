-- name: InsertBoundarySession :one
INSERT INTO boundary_sessions (
    id,
    workspace_agent_id,
    owner_id,
    confined_process_name,
    started_at,
    updated_at
) VALUES (
    @id,
    @workspace_agent_id,
    @owner_id,
    @confined_process_name,
    @started_at,
    @updated_at
) RETURNING *;

-- name: GetBoundarySessionByID :one
SELECT * FROM boundary_sessions WHERE id = @id;

-- name: InsertBoundaryLog :one
INSERT INTO boundary_logs (
    id,
    session_id,
    sequence_number,
    captured_at,
    created_at,
    proto,
    method,
    detail,
    matched_rule
) VALUES (
    @id,
    @session_id,
    @sequence_number,
    @captured_at,
    @created_at,
    @proto,
    @method,
    @detail,
    @matched_rule
) RETURNING *;

-- name: InsertBoundaryLogs :many
INSERT INTO boundary_logs (
    id,
    session_id,
    sequence_number,
    captured_at,
    created_at,
    proto,
    method,
    detail,
    matched_rule
)
SELECT
    unnest(@id :: uuid[]),
    @session_id :: uuid,
    unnest(@sequence_number :: int[]),
    unnest(@captured_at :: timestamptz[]),
    unnest(@created_at :: timestamptz[]),
    unnest(@proto :: text[]),
    unnest(@method :: text[]),
    unnest(@detail :: text[]),
    unnest(@matched_rule :: text[])
RETURNING *;

-- name: GetBoundaryLogByID :one
SELECT * FROM boundary_logs WHERE id = @id;

-- name: ListBoundaryLogsBySessionID :many
-- Lists boundary logs for a session, sorted by sequence number ascending.
-- Supports optional exclusive sequence number bounds (seq_after, seq_before)
-- for fetching events between two known interceptions.
SELECT *
FROM boundary_logs
WHERE
    session_id = @session_id
    AND CASE
        WHEN sqlc.narg('seq_after')::int IS NOT NULL THEN sequence_number > sqlc.narg('seq_after')
        ELSE true
    END
    AND CASE
        WHEN sqlc.narg('seq_before')::int IS NOT NULL THEN sequence_number < sqlc.narg('seq_before')
        ELSE true
    END
ORDER BY sequence_number ASC
LIMIT COALESCE(NULLIF(@limit_opt::int, 0), 100);

-- name: DeleteOldBoundaryLogs :execrows
-- Deletes boundary logs older than the given time, bounded by a row limit
-- to avoid long-running transactions.
WITH old_logs AS (
    SELECT id
    FROM boundary_logs
    WHERE captured_at < @before_time::timestamptz
    ORDER BY captured_at ASC
    LIMIT @limit_count
)
DELETE FROM boundary_logs
USING old_logs
WHERE boundary_logs.id = old_logs.id;
