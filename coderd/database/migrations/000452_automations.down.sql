ALTER TABLE chats DROP COLUMN IF EXISTS automation_id;

DROP TABLE IF EXISTS automation_events;

DROP TABLE IF EXISTS automation_triggers;

DROP TABLE IF EXISTS automations;
