ALTER TABLE chats ADD COLUMN file_ids uuid[] DEFAULT '{}'::uuid[] NOT NULL;
