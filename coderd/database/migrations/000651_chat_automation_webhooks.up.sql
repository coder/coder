-- The new values are not used in this migration: an enum value cannot be
-- used in the transaction that adds it.
ALTER TYPE chat_automation_event_delivery ADD VALUE IF NOT EXISTS 'webhook';
ALTER TYPE chat_automation_subscription_state ADD VALUE IF NOT EXISTS 'pending';

ALTER TABLE chat_automation_mcp_subscriptions
    ADD COLUMN webhook_subscription_id text,
    ADD COLUMN refresh_before timestamptz,
    ADD COLUMN renew_at timestamptz,
    ADD CONSTRAINT chat_automation_mcp_subscriptions_webhook_shape CHECK (delivery = 'poll' OR state <> 'active' OR renew_at IS NOT NULL);

COMMENT ON COLUMN chat_automation_mcp_subscriptions.webhook_subscription_id IS 'The id that the MCP server derived for a webhook subscription and sends in X-MCP-Subscription-Id.';
COMMENT ON COLUMN chat_automation_mcp_subscriptions.refresh_before IS 'When the MCP server''s grant of a webhook subscription expires.';
COMMENT ON COLUMN chat_automation_mcp_subscriptions.renew_at IS 'When a webhook subscription is next subscribed again. A claim moves it ahead as a lease.';

CREATE INDEX chat_automation_mcp_subscriptions_renew_at_idx ON chat_automation_mcp_subscriptions (renew_at) WHERE renew_at IS NOT NULL;

-- Its own table, so that only the receiver and the renewal read the secret.
CREATE TABLE chat_automation_webhook_secrets (
    automation_id uuid PRIMARY KEY REFERENCES chat_automation_mcp_subscriptions (automation_id) ON DELETE CASCADE,
    secret text NOT NULL,
    secret_key_id text REFERENCES dbcrypt_keys (active_key_digest)
);

COMMENT ON TABLE chat_automation_webhook_secrets IS 'The Standard Webhooks secret that signs the deliveries of a webhook subscription.';
COMMENT ON COLUMN chat_automation_webhook_secrets.secret_key_id IS 'The ID of the key used to encrypt the secret. If this is NULL, the secret is not encrypted.';
