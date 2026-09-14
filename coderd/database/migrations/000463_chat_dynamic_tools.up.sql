ALTER TYPE chat_status ADD VALUE IF NOT EXISTS 'requires_action';

ALTER TABLE chats ADD COLUMN dynamic_tools JSONB DEFAULT NULL;
