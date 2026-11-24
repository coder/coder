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
ALTER TABLE chat_automation_mcp_subscriptions
    DROP CONSTRAINT chat_automation_mcp_subscriptions_webhook_shape,
    DROP COLUMN webhook_subscription_id,
    DROP COLUMN refresh_before,
    DROP COLUMN renew_at;
