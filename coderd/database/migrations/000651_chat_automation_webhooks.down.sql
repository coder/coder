-- The webhook and pending enum values are left in place; enum values cannot
-- be dropped. Their automations go, since the previous release cannot run
-- them.
DELETE FROM chat_automations
WHERE id IN (
    SELECT automation_id
    FROM chat_automation_mcp_subscriptions
    WHERE delivery::text = 'webhook' OR state::text = 'pending'
);
DROP TABLE chat_automation_webhook_secrets;
-- Generations may repeat an event ID; the oldest row of each stays.
DELETE FROM chat_automation_event_inbox a
USING chat_automation_event_inbox b
WHERE a.automation_id = b.automation_id AND a.event_id = b.event_id AND a.id > b.id;
DROP INDEX chat_automation_event_inbox_event_id_idx;
CREATE UNIQUE INDEX chat_automation_event_inbox_event_id_idx ON chat_automation_event_inbox (automation_id, event_id) WHERE event_id IS NOT NULL;
ALTER TABLE chat_automation_event_inbox DROP COLUMN generation;
ALTER TABLE chat_automation_mcp_subscriptions
    DROP CONSTRAINT chat_automation_mcp_subscriptions_webhook_shape,
    DROP COLUMN webhook_subscription_id,
    DROP COLUMN refresh_before,
    DROP COLUMN renew_at,
    DROP COLUMN generation;
