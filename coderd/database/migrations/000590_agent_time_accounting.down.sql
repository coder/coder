-- Rolling back capture also ends the deletion-preservation guarantee.
DROP TRIGGER IF EXISTS trigger_agent_time_preserve_chat_before_delete ON chats;
DROP TRIGGER IF EXISTS trigger_agent_time_account_chat_messages_after_insert ON chat_messages;

DROP FUNCTION IF EXISTS agent_time_preserve_chat_before_delete();
DROP FUNCTION IF EXISTS agent_time_account_chat_messages_after_insert();
DROP FUNCTION IF EXISTS account_agent_time_messages(bigint[], uuid, uuid, uuid);
DROP FUNCTION IF EXISTS agent_time_delete_fallback_limit();

DROP INDEX IF EXISTS idx_chat_messages_agent_time_lookup;

DROP TABLE IF EXISTS agent_time_backfill_status;
DROP TABLE IF EXISTS chat_message_agent_time_accounted;
DROP TABLE IF EXISTS agent_time_organization_daily;
DROP TABLE IF EXISTS agent_time_daily;
DROP TABLE IF EXISTS agent_time_capture;
