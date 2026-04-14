ALTER TABLE chats ADD COLUMN ancestor_chat_id UUID REFERENCES chats(id) ON DELETE SET NULL;
ALTER TABLE chats ADD COLUMN ancestor_message_id BIGINT;
ALTER TABLE chats ADD CONSTRAINT chats_ancestor_message_requires_chat CHECK (ancestor_message_id IS NULL OR ancestor_chat_id IS NOT NULL);
