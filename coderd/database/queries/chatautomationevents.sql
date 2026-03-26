-- name: InsertChatAutomationEvent :one
INSERT INTO chat_automation_events (
    id,
    automation_id,
    trigger_id,
    received_at,
    payload,
    filter_matched,
    resolved_labels,
    matched_chat_id,
    created_chat_id,
    status,
    error
) VALUES (
    @id::uuid,
    @automation_id::uuid,
    sqlc.narg('trigger_id')::uuid,
    @received_at::timestamptz,
    @payload::jsonb,
    @filter_matched::boolean,
    sqlc.narg('resolved_labels')::jsonb,
    sqlc.narg('matched_chat_id')::uuid,
    sqlc.narg('created_chat_id')::uuid,
    @status::chat_automation_event_status,
    sqlc.narg('error')::text
) RETURNING *;

-- name: GetChatAutomationEvents :many
SELECT
    *
FROM
    chat_automation_events
WHERE
    automation_id = @automation_id::uuid
    AND CASE
        WHEN sqlc.narg('status_filter')::chat_automation_event_status IS NOT NULL THEN status = sqlc.narg('status_filter')::chat_automation_event_status
        ELSE true
    END
ORDER BY
    received_at DESC
OFFSET @offset_opt
LIMIT
    COALESCE(NULLIF(@limit_opt :: int, 0), 50);

-- name: CountChatAutomationChatCreatesInWindow :one
-- Counts new-chat events in the rate-limit window. This count is
-- approximate under concurrency: concurrent webhook handlers may
-- each read the same count before any of them insert, so brief
-- bursts can slightly exceed the configured cap.
SELECT COUNT(*)
FROM chat_automation_events
WHERE automation_id = @automation_id::uuid
    AND status = 'created'
    AND received_at > @window_start::timestamptz;

-- name: CountChatAutomationMessagesInWindow :one
SELECT COUNT(*)
FROM chat_automation_events
WHERE automation_id = @automation_id::uuid
    AND status IN ('created', 'continued')
    AND received_at > @window_start::timestamptz;

-- name: PurgeOldChatAutomationEvents :execrows
-- Deletes old chat automation events in bounded batches to avoid
-- long-running locks on high-volume tables. Callers should loop
-- until zero rows are returned.
WITH old_events AS (
    SELECT id
    FROM chat_automation_events
    WHERE received_at < @before::timestamptz
    ORDER BY received_at ASC
    LIMIT @limit_count
)
DELETE FROM chat_automation_events
USING old_events
WHERE chat_automation_events.id = old_events.id;
