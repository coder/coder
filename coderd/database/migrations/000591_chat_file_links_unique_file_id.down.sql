CREATE INDEX idx_chat_file_links_file_id ON chat_file_links (file_id);

ALTER TABLE chat_file_links
    DROP CONSTRAINT chat_file_links_file_id_key;
