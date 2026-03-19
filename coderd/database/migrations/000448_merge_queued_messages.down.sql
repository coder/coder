-- Recreate the chat_queued_messages table.
CREATE TABLE chat_queued_messages (
    id bigint NOT NULL,
    chat_id uuid NOT NULL,
    content jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE SEQUENCE chat_queued_messages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE chat_queued_messages_id_seq OWNED BY chat_queued_messages.id;
ALTER TABLE ONLY chat_queued_messages ALTER COLUMN id SET DEFAULT nextval('chat_queued_messages_id_seq'::regclass);
ALTER TABLE ONLY chat_queued_messages ADD CONSTRAINT chat_queued_messages_pkey PRIMARY KEY (id);
CREATE INDEX idx_chat_queued_messages_chat_id ON chat_queued_messages USING btree (chat_id);
ALTER TABLE ONLY chat_queued_messages ADD CONSTRAINT chat_queued_messages_chat_id_fkey FOREIGN KEY (chat_id) REFERENCES chats(id) ON DELETE CASCADE;

-- Migrate queued messages back.
INSERT INTO chat_queued_messages (chat_id, content, created_at)
SELECT chat_id, content, created_at
FROM chat_messages
WHERE queued = true
ORDER BY id ASC;

DELETE FROM chat_messages WHERE queued = true;
ALTER TABLE chat_messages DROP COLUMN queued;
