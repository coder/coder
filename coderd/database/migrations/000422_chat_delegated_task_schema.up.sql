ALTER TABLE chats
    ADD COLUMN parent_chat_id UUID REFERENCES chats(id) ON DELETE SET NULL,
    ADD COLUMN root_chat_id UUID REFERENCES chats(id) ON DELETE SET NULL;

-- Existing chats are non-delegated; make them their own root.
UPDATE chats
SET
    root_chat_id = id
WHERE
    root_chat_id IS NULL;

CREATE INDEX idx_chats_parent_chat_id ON chats(parent_chat_id);
CREATE INDEX idx_chats_root_chat_id ON chats(root_chat_id);
