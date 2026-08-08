CREATE TYPE chat_mode AS ENUM ('computer_use');

ALTER TABLE chats ADD COLUMN mode chat_mode;
