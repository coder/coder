CREATE TABLE chat_git_changes (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    chat_id         UUID        NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    file_path       TEXT        NOT NULL,
    change_type     TEXT        NOT NULL, -- 'added', 'modified', 'deleted', 'renamed'
    old_path        TEXT,                 -- For renames
    diff_summary    TEXT,                 -- Optional: lines added/removed summary
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE(chat_id, file_path)
);

CREATE INDEX idx_chat_git_changes_chat ON chat_git_changes(chat_id);
