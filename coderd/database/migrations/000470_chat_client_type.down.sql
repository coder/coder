ALTER TABLE chats DROP COLUMN IF EXISTS client_type;

DROP TYPE IF EXISTS chat_client_type;
