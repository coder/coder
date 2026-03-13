ALTER TABLE chat_diff_statuses ADD COLUMN author_login TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_diff_statuses ADD COLUMN author_avatar_url TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_diff_statuses ADD COLUMN base_branch TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_diff_statuses ADD COLUMN pr_number INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_diff_statuses ADD COLUMN commits INTEGER NOT NULL DEFAULT 0;
ALTER TABLE chat_diff_statuses ADD COLUMN approved BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE chat_diff_statuses ADD COLUMN reviewer_count INTEGER NOT NULL DEFAULT 0;
