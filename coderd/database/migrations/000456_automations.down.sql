ALTER TABLE chats DROP COLUMN IF EXISTS automation_id;

DROP TABLE IF EXISTS chat_automation_events;

DROP TABLE IF EXISTS chat_automation_triggers;

DROP TABLE IF EXISTS chat_automations;

DROP TYPE IF EXISTS chat_automation_event_status;

DROP TYPE IF EXISTS chat_automation_trigger_type;

DROP TYPE IF EXISTS chat_automation_status;
