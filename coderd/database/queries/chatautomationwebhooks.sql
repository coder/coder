-- name: InsertChatAutomationWebhookSecret :exec
INSERT INTO chat_automation_webhook_secrets (automation_id, secret, secret_key_id)
VALUES (@automation_id::uuid, @secret::text, sqlc.narg('secret_key_id')::text);

-- name: GetChatAutomationWebhookSecret :one
SELECT *
FROM chat_automation_webhook_secrets
WHERE automation_id = @automation_id::uuid;

-- name: GetChatAutomationWebhookSecrets :many
SELECT *
FROM chat_automation_webhook_secrets
ORDER BY automation_id;

-- name: UpdateEncryptedChatAutomationWebhookSecret :exec
UPDATE chat_automation_webhook_secrets
SET secret = @secret::text, secret_key_id = sqlc.narg('secret_key_id')::text
WHERE automation_id = @automation_id::uuid;

-- name: ClaimDueChatAutomationWebhookRenewals :many
-- Leases the pending and active webhook subscriptions of enabled automations
-- whose renew_at has passed by moving renew_at to @lease_until.
WITH due AS (
    SELECT s.automation_id
    FROM chat_automation_mcp_subscriptions s
    JOIN chat_automations a ON a.id = s.automation_id
    WHERE
        s.delivery = 'webhook'
        AND s.state <> 'terminated'
        AND s.renew_at <= @now::timestamptz
        AND a.enabled
    ORDER BY s.renew_at
    LIMIT @limit_count::int
    FOR UPDATE OF s SKIP LOCKED
)
UPDATE chat_automation_mcp_subscriptions s
SET renew_at = @lease_until::timestamptz
FROM due
WHERE s.automation_id = due.automation_id
RETURNING s.*;

-- name: UpdateChatAutomationWebhookGrant :execrows
-- Stores what an events/subscribe call returned and activates the
-- subscription, unless renew_at moved off @lease during the call, as a reset
-- or a re-enable does. A NULL cursor keeps the stored one.
UPDATE chat_automation_mcp_subscriptions
SET
    state = 'active',
    webhook_subscription_id = @webhook_subscription_id::text,
    refresh_before = @refresh_before::timestamptz,
    renew_at = @renew_at::timestamptz,
    cursor = COALESCE(sqlc.narg('cursor')::text, cursor),
    last_error = sqlc.narg('last_error')::text,
    updated_at = @now::timestamptz
WHERE automation_id = @automation_id::uuid AND delivery = 'webhook' AND state <> 'terminated'
    AND renew_at = @lease::timestamptz;

-- name: ResetChatAutomationMCPSubscriptions :exec
-- Restarts the subscriptions to an MCP server config whose server, principal
-- or enabled state changed. Each resumes from now in a new generation, so
-- that another server's event IDs cannot match the old server's, and a poll
-- in flight changes nothing. An enabled poll subscription that had a cursor
-- gets a gap row for the events since its last poll. Each webhook grant
-- ends, so the receiver refuses the old server and the next tick subscribes
-- again. Rows lock in automation_id order.
WITH due AS (
    SELECT s.automation_id, s.delivery = 'poll' AND s.cursor IS NOT NULL AND a.enabled AS missed
    FROM chat_automation_mcp_subscriptions s
    JOIN chat_automations a ON a.id = s.automation_id
    WHERE s.mcp_server_config_id = @mcp_server_config_id::uuid AND s.state <> 'terminated'
    ORDER BY s.automation_id
    FOR UPDATE OF s
), reset AS (
    UPDATE chat_automation_mcp_subscriptions s
    SET
        cursor = NULL,
        generation = s.generation + 1,
        next_poll_at = CASE WHEN s.delivery = 'poll' THEN now() END,
        refresh_before = CASE WHEN s.delivery = 'webhook' THEN now() ELSE s.refresh_before END,
        renew_at = CASE WHEN s.delivery = 'webhook' THEN now() ELSE s.renew_at END,
        updated_at = now()
    FROM due
    WHERE s.automation_id = due.automation_id
    RETURNING s.automation_id, s.generation, due.missed
)
INSERT INTO chat_automation_event_inbox (automation_id, generation, kind, detail)
SELECT automation_id, generation, 'gap', 'Some events are missing. The MCP server config changed, so the events since the last poll were not fetched.'
FROM reset
WHERE missed;

-- name: SetChatAutomationWebhookRenewAt :exec
-- Schedules the next subscribe of a webhook subscription, after an error or
-- to subscribe again at once. A NULL error clears it.
UPDATE chat_automation_mcp_subscriptions
SET renew_at = @renew_at::timestamptz, last_error = sqlc.narg('last_error')::text, updated_at = @now::timestamptz
WHERE automation_id = @automation_id::uuid AND delivery = 'webhook' AND state <> 'terminated';

-- name: CountPendingChatAutomationInboxRows :one
-- Counts the undelivered inbox rows of a subscription, up to @max_pending.
SELECT count(*)::int
FROM (
    SELECT 1
    FROM chat_automation_event_inbox
    WHERE automation_id = @automation_id::uuid AND delivered_at IS NULL
    LIMIT @max_pending::int
) pending;
