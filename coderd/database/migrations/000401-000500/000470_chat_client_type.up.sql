CREATE TYPE chat_client_type AS ENUM (
    'ui',
    'api'
);

ALTER TABLE chats ADD COLUMN client_type chat_client_type NOT NULL DEFAULT 'api'::chat_client_type;

-- Backfill all existing rows to 'ui' since they were created
-- from the web interface before this column existed.
UPDATE chats SET client_type = 'ui';
