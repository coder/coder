CREATE TABLE chat_file_links (
    chat_id uuid NOT NULL,
    file_id uuid NOT NULL,
    UNIQUE (chat_id, file_id)
);

CREATE INDEX idx_chat_file_links_chat_id ON chat_file_links (chat_id);

ALTER TABLE chats DROP COLUMN IF EXISTS file_ids;
