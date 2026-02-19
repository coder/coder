CREATE TYPE chat_task_status AS ENUM (
    'queued',
    'running',
    'awaiting_report',
    'reported'
);

ALTER TABLE chats
    ADD COLUMN parent_chat_id UUID REFERENCES chats(id) ON DELETE SET NULL,
    ADD COLUMN root_chat_id UUID REFERENCES chats(id) ON DELETE SET NULL,
    ADD COLUMN task_status chat_task_status,
    ADD COLUMN task_report TEXT;

-- Existing chats are non-delegated; make them their own root and reported.
UPDATE chats
SET
    root_chat_id = id,
    task_status = 'reported'::chat_task_status
WHERE
    root_chat_id IS NULL
    OR task_status IS NULL;

ALTER TABLE chats
    ALTER COLUMN task_status SET DEFAULT 'reported'::chat_task_status,
    ALTER COLUMN task_status SET NOT NULL;

CREATE INDEX idx_chats_parent_chat_id ON chats(parent_chat_id);
CREATE INDEX idx_chats_root_chat_id ON chats(root_chat_id);
