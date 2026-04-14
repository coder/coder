ALTER TABLE chats DROP CONSTRAINT IF EXISTS chats_ancestor_message_requires_chat;
ALTER TABLE chats DROP COLUMN ancestor_message_id;
ALTER TABLE chats DROP COLUMN ancestor_chat_id;
