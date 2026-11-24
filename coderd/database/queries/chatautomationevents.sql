-- name: InsertChatAutomationMCPSubscription :one
INSERT INTO chat_automation_mcp_subscriptions (automation_id, mcp_server_config_id, event_name, arguments, delivery, state, cursor, next_poll_at, renew_at)
VALUES (
    @automation_id::uuid, @mcp_server_config_id::uuid, @event_name::text, @arguments::jsonb,
    @delivery::chat_automation_event_delivery, @state::chat_automation_subscription_state,
    sqlc.narg('cursor')::text, sqlc.narg('next_poll_at')::timestamptz, sqlc.narg('renew_at')::timestamptz
)
RETURNING *;

-- name: GetChatAutomationMCPSubscriptionsByAutomationIDs :many
SELECT *
FROM chat_automation_mcp_subscriptions
WHERE automation_id = ANY(@automation_ids::uuid[]);

-- name: ClaimDueChatAutomationMCPPolls :many
-- Leases due active poll subscriptions of enabled automations with fewer
-- than @max_pending undelivered inbox rows by moving next_poll_at to
-- @lease_until. SKIP LOCKED lets every replica claim a different set.
WITH due AS (
    SELECT s.automation_id
    FROM chat_automation_mcp_subscriptions s
    JOIN chat_automations a ON a.id = s.automation_id
    WHERE
        s.state = 'active'
        AND s.delivery = 'poll'
        AND s.next_poll_at <= @now::timestamptz
        AND a.enabled
        AND (
            SELECT count(*)
            FROM (
                SELECT 1
                FROM chat_automation_event_inbox i
                WHERE i.automation_id = s.automation_id AND i.delivered_at IS NULL
                LIMIT @max_pending::int
            ) pending
        ) < @max_pending::int
    ORDER BY s.next_poll_at
    LIMIT @limit_count::int
    FOR UPDATE OF s SKIP LOCKED
)
UPDATE chat_automation_mcp_subscriptions s
SET next_poll_at = @lease_until::timestamptz
FROM due, chat_automations a
WHERE s.automation_id = due.automation_id AND a.id = s.automation_id
RETURNING s.*, a.owner_id;

-- name: ClaimChatAutomationInboxDrains :many
-- Leases subscriptions of enabled automations with undelivered inbox rows
-- whose drain_after has passed by moving drain_after to @lease_until.
WITH due AS (
    SELECT s.automation_id
    FROM chat_automation_mcp_subscriptions s
    JOIN chat_automations a ON a.id = s.automation_id
    WHERE
        s.drain_after <= @now::timestamptz
        AND a.enabled
        AND EXISTS (
            SELECT 1
            FROM chat_automation_event_inbox i
            WHERE i.automation_id = s.automation_id AND i.delivered_at IS NULL
        )
    ORDER BY s.drain_after
    LIMIT @limit_count::int
    FOR UPDATE OF s SKIP LOCKED
)
UPDATE chat_automation_mcp_subscriptions s
SET drain_after = @lease_until::timestamptz
FROM due
WHERE s.automation_id = due.automation_id
RETURNING s.*;

-- name: SetChatAutomationMCPDrainAfter :exec
UPDATE chat_automation_mcp_subscriptions
SET drain_after = @drain_after::timestamptz
WHERE automation_id = @automation_id::uuid;

-- name: UpdateChatAutomationMCPCursor :execrows
-- Records a delivery or a failed poll of an active subscription of an enabled
-- automation: the cursor to resume from, the next poll and the error. A NULL
-- cursor or next poll keeps the stored one; a NULL error clears it.
UPDATE chat_automation_mcp_subscriptions s
SET
    cursor = COALESCE(sqlc.narg('cursor')::text, s.cursor),
    next_poll_at = COALESCE(sqlc.narg('next_poll_at')::timestamptz, s.next_poll_at),
    last_error = sqlc.narg('last_error')::text,
    updated_at = @now::timestamptz
FROM chat_automations a
WHERE s.automation_id = @automation_id::uuid AND s.state = 'active' AND a.id = s.automation_id AND a.enabled;

-- name: TerminateChatAutomationMCPSubscription :execrows
UPDATE chat_automation_mcp_subscriptions
SET state = 'terminated', last_error = @reason::text, updated_at = @now::timestamptz
WHERE automation_id = @automation_id::uuid AND state <> 'terminated';

-- name: InsertChatAutomationInboxEvents :execrows
-- Appends @events, a JSON array of MCP event occurrences, in their order. An
-- eventId that the subscription already received inserts nothing.
INSERT INTO chat_automation_event_inbox (automation_id, kind, event_id, event_name, occurred_at, data)
SELECT
    @automation_id::uuid,
    'event',
    e.value->>'eventId',
    e.value->>'name',
    (e.value->>'timestamp')::timestamptz,
    COALESCE(NULLIF(e.value->'data', 'null'::jsonb), '{}'::jsonb)
FROM jsonb_array_elements(@events::jsonb) WITH ORDINALITY AS e(value, ord)
ORDER BY e.ord
ON CONFLICT (automation_id, event_id) WHERE event_id IS NOT NULL DO NOTHING;

-- name: InsertChatAutomationInboxNotice :exec
-- A non-NULL @event_id deduplicates a retried delivery of the notice.
INSERT INTO chat_automation_event_inbox (automation_id, kind, event_id, detail)
VALUES (@automation_id::uuid, @kind::chat_automation_inbox_kind, sqlc.narg('event_id')::text, @detail::text)
ON CONFLICT (automation_id, event_id) WHERE event_id IS NOT NULL DO NOTHING;

-- name: GetPendingChatAutomationInboxRows :many
SELECT *
FROM chat_automation_event_inbox
WHERE automation_id = @automation_id::uuid AND delivered_at IS NULL
ORDER BY id
LIMIT @limit_count::int;

-- name: MarkChatAutomationInboxDelivered :execrows
UPDATE chat_automation_event_inbox
SET delivered_at = @delivered_at::timestamptz
WHERE automation_id = @automation_id::uuid AND id = ANY(@ids::bigint[]) AND delivered_at IS NULL;

-- name: DeleteOldChatAutomationInboxRows :execrows
-- Delivered rows are kept for a while so that a late retry of an event
-- still deduplicates.
DELETE FROM chat_automation_event_inbox
WHERE id IN (
    SELECT id
    FROM chat_automation_event_inbox
    WHERE delivered_at < @before_time::timestamptz
    ORDER BY id
    LIMIT @limit_count::int
);
