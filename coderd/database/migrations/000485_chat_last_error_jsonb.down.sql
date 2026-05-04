ALTER TABLE chats
  ALTER COLUMN last_error TYPE text
  USING last_error ->> 'message';
