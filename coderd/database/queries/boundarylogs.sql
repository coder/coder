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
SELECT
    bs.*,
    w.id AS workspace_id,
    w.owner_id AS workspace_owner_id
FROM
    boundary_sessions bs
JOIN
    workspace_agents wa ON wa.id = bs.workspace_agent_id
JOIN
    workspace_resources wr ON wr.id = wa.resource_id
JOIN
    workspace_builds wb ON wb.job_id = wr.job_id
JOIN
    workspaces w ON w.id = wb.workspace_id
WHERE
    bs.id = @id;

-- name: InsertBoundaryLogs :many
INSERT INTO boundary_logs (
    id,
    session_id,
    owner_id,
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
    @owner_id :: uuid,
    unnest(@sequence_number :: int[]),
    unnest(@captured_at :: timestamptz[]),
    unnest(@created_at :: timestamptz[]),
    unnest(@proto :: text[]),
    unnest(@method :: text[]),
    unnest(@detail :: text[]),
    NULLIF(unnest(@matched_rule :: text[]), '')
RETURNING *;

-- name: GetBoundaryLogByID :one
SELECT * FROM boundary_logs WHERE id = @id;

-- name: ListBoundaryLogsBySessionID :many
-- Lists boundary logs for a session, sorted by sequence number ascending.
-- Supports an inclusive lower bound (seq_after) and an exclusive upper bound
-- (seq_before) for fetching events between two known interceptions.
SELECT *
FROM boundary_logs
WHERE
    session_id = @session_id
    AND CASE
        WHEN sqlc.narg('seq_after')::int IS NOT NULL THEN sequence_number >= sqlc.narg('seq_after')
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

-- name: DeleteOldBoundarySessions :execrows
-- Deletes boundary sessions that have aged past retention and no longer
-- have any associated logs.
WITH old_sessions AS (
    SELECT bs.id
    FROM boundary_sessions bs
    WHERE bs.updated_at < @before_time::timestamptz
      AND NOT EXISTS (
          SELECT 1 FROM boundary_logs bl WHERE bl.session_id = bs.id
      )
    ORDER BY bs.updated_at ASC
    LIMIT @limit_count
)
DELETE FROM boundary_sessions
USING old_sessions
WHERE boundary_sessions.id = old_sessions.id;
