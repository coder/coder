CREATE TABLE chat_file_links (
    chat_id uuid NOT NULL,
    file_id uuid NOT NULL,
    UNIQUE (chat_id, file_id)
);

CREATE INDEX idx_chat_file_links_chat_id ON chat_file_links (chat_id);

ALTER TABLE chat_file_links
    ADD CONSTRAINT chat_file_links_chat_id_fkey
    FOREIGN KEY (chat_id) REFERENCES chats(id) ON DELETE CASCADE;

ALTER TABLE chat_file_links
    ADD CONSTRAINT chat_file_links_file_id_fkey
    FOREIGN KEY (file_id) REFERENCES chat_files(id) ON DELETE CASCADE;

ALTER TABLE chats DROP COLUMN IF EXISTS file_ids;
