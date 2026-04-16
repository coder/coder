CREATE TYPE chat_runner_type AS ENUM ('coderd', 'workspace_agent');
ALTER TABLE chats ADD COLUMN runner_type chat_runner_type;
