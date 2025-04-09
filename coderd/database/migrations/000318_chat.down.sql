-- Remove the created_at column
ALTER TABLE chat_messages
DROP COLUMN IF EXISTS created_at;

-- Add back message_type and message_index (assuming previous state was TEXT and SERIAL)
ALTER TABLE chat_messages
ADD COLUMN IF NOT EXISTS message_type TEXT NOT NULL DEFAULT '', -- Provide a default or handle NULLs
ADD COLUMN IF NOT EXISTS message_index SERIAL;

-- Change content back to TEXT (data loss may occur if JSONB data is not representable as TEXT)
ALTER TABLE chat_messages
ALTER COLUMN content TYPE TEXT USING content::TEXT;

-- Attempt to revert id back to UUID with default
-- WARNING: This is complex and potentially destructive. It might fail if data exists.
-- It drops the existing primary key, sequence, and default, then attempts to set a new one.
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_pkey;
DROP SEQUENCE IF EXISTS chat_messages_id_seq;
ALTER TABLE chat_messages ALTER COLUMN id DROP DEFAULT;
ALTER TABLE chat_messages ALTER COLUMN id TYPE UUID USING (gen_random_uuid()); -- Attempt conversion, may fail
ALTER TABLE chat_messages ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE chat_messages ADD PRIMARY KEY (id);

-- Revert changes to chat_conversations (removing deleted_at)
ALTER TABLE chat_conversations
DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE chat_conversations DROP CONSTRAINT IF EXISTS chat_conversations_user_id_fkey;
ALTER TABLE chat_conversations ADD CONSTRAINT chat_conversations_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);
ALTER TABLE chat_conversations ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE chat_conversations ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP;

-- Revert changes to chat_messages (removing ON DELETE CASCADE)
ALTER TABLE chat_messages DROP CONSTRAINT IF EXISTS chat_messages_conversation_id_fkey;
ALTER TABLE chat_messages ADD CONSTRAINT chat_messages_conversation_id_fkey FOREIGN KEY (conversation_id) REFERENCES chat_conversations(id);
