CREATE TABLE chat_notes (
    chat_id uuid NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    path text NOT NULL CHECK (path ~ '^[a-z0-9][a-z0-9._/-]{0,127}$'),
    content text NOT NULL CHECK (octet_length(content) <= 16384),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (chat_id, path)
);

COMMENT ON TABLE chat_notes IS 'Chat-scoped notes that an agent keeps across context windows.';
