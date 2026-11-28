-- The mcp_event chat_automation_kind value is left in place; enum values
-- cannot be dropped. Its automations go, since the previous release cannot
-- run them.
DELETE FROM chat_automations WHERE kind = 'mcp_event';
DROP TABLE chat_automation_event_inbox;
DROP TABLE chat_automation_mcp_subscriptions;
DROP TYPE chat_automation_inbox_kind;
DROP TYPE chat_automation_subscription_state;
DROP TYPE chat_automation_event_delivery;
