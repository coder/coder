DROP INDEX IF EXISTS idx_chats_automation;
ALTER TABLE chats DROP COLUMN IF EXISTS automation_id;

DROP TABLE IF EXISTS chat_automation_runs;
DROP TABLE IF EXISTS chat_automations;
