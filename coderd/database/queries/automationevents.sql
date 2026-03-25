-- name: InsertAutomationEvent :one
INSERT INTO automation_events (
    automation_id,
    trigger_id,
    payload,
    filter_matched,
    resolved_labels,
    matched_chat_id,
    created_chat_id,
    status,
    error
) VALUES (
    @automation_id::uuid,
    sqlc.narg('trigger_id')::uuid,
    @payload::jsonb,
    @filter_matched::boolean,
    sqlc.narg('resolved_labels')::jsonb,
    sqlc.narg('matched_chat_id')::uuid,
    sqlc.narg('created_chat_id')::uuid,
    @status::text,
    sqlc.narg('error')::text
) RETURNING *;

-- name: GetAutomationEvents :many
SELECT
    *
FROM
    automation_events
WHERE
    automation_id = @automation_id::uuid
    AND CASE
        WHEN sqlc.narg('status_filter')::text IS NOT NULL THEN status = sqlc.narg('status_filter')::text
        ELSE true
    END
ORDER BY
    received_at DESC
OFFSET @offset_opt
LIMIT
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: CountAutomationChatCreatesInWindow :one
SELECT COUNT(*)
FROM automation_events
WHERE automation_id = @automation_id::uuid
    AND status = 'created'
    AND received_at > @window_start::timestamptz;

-- name: CountAutomationMessagesInWindow :one
SELECT COUNT(*)
FROM automation_events
WHERE automation_id = @automation_id::uuid
    AND status = 'continued'
    AND received_at > @window_start::timestamptz;

-- name: PurgeOldAutomationEvents :exec
DELETE FROM automation_events
WHERE received_at < NOW() - INTERVAL '7 days';
