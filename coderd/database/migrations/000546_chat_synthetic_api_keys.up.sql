CREATE TABLE chat_synthetic_api_keys (
    user_id uuid NOT NULL PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    api_key_id text NOT NULL UNIQUE REFERENCES api_keys(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE chat_messages
DROP CONSTRAINT chat_messages_api_key_id_fkey;

ALTER TABLE chat_queued_messages
DROP CONSTRAINT chat_queued_messages_api_key_id_fkey;
