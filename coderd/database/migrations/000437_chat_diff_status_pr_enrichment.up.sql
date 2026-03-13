ALTER TABLE chat_diff_statuses ADD COLUMN author_login TEXT;
ALTER TABLE chat_diff_statuses ADD COLUMN author_avatar_url TEXT;
ALTER TABLE chat_diff_statuses ADD COLUMN base_branch TEXT;
ALTER TABLE chat_diff_statuses ADD COLUMN pr_number INTEGER;
ALTER TABLE chat_diff_statuses ADD COLUMN commits INTEGER;
ALTER TABLE chat_diff_statuses ADD COLUMN approved BOOLEAN;
ALTER TABLE chat_diff_statuses ADD COLUMN reviewer_count INTEGER;
