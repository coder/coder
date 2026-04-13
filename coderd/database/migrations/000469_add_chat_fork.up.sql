ALTER TABLE chats ADD COLUMN ancestor_chat_id UUID REFERENCES chats(id) ON DELETE SET NULL;
ALTER TABLE chats ADD COLUMN ancestor_message_id BIGINT;
