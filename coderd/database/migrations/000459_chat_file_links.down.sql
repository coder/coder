ALTER TABLE chats ADD COLUMN file_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL;

UPDATE chats SET file_ids = (
    SELECT COALESCE(array_agg(cfl.file_id), '{}')
    FROM chat_file_links cfl
    WHERE cfl.chat_id = chats.id
);

DROP TABLE chat_file_links;
