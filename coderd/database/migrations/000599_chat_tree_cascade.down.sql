DROP FUNCTION IF EXISTS chat_subtree(uuid);

DROP INDEX IF EXISTS idx_chats_tree_adoptable;

ALTER TABLE chats
    DROP CONSTRAINT chats_parent_chat_id_fkey,
    DROP CONSTRAINT chats_root_chat_id_fkey;

ALTER TABLE chats
    ADD CONSTRAINT chats_parent_chat_id_fkey
        FOREIGN KEY (parent_chat_id) REFERENCES chats(id) ON DELETE SET NULL,
    ADD CONSTRAINT chats_root_chat_id_fkey
        FOREIGN KEY (root_chat_id) REFERENCES chats(id) ON DELETE SET NULL;
