CREATE TABLE chat_diff_statuses (
    chat_id             UUID        PRIMARY KEY REFERENCES chats(id) ON DELETE CASCADE,
    github_pr_url       TEXT,
    pull_request_state  TEXT        NOT NULL DEFAULT '',
    pull_request_open   BOOLEAN     NOT NULL DEFAULT FALSE,
    changes_requested   BOOLEAN     NOT NULL DEFAULT FALSE,
    additions           INTEGER     NOT NULL DEFAULT 0,
    deletions           INTEGER     NOT NULL DEFAULT 0,
    changed_files       INTEGER     NOT NULL DEFAULT 0,
    refreshed_at        TIMESTAMPTZ,
    stale_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_chat_diff_statuses_stale_at ON chat_diff_statuses(stale_at);
