CREATE TYPE chat_plan_mode AS ENUM ('plan');
ALTER TABLE chats ADD COLUMN plan_mode chat_plan_mode;
