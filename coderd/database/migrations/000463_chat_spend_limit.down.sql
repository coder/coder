ALTER TABLE chats DROP CONSTRAINT IF EXISTS chats_spend_limit_micros_positive;
ALTER TABLE chats DROP COLUMN IF EXISTS spend_limit_micros;
