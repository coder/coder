DROP TABLE IF EXISTS chat_debug_steps;
DROP TABLE IF EXISTS chat_debug_runs;
ALTER TABLE chats DROP COLUMN debug_logs_enabled_override;
