-- One row per chat, origin, and branch. This lets a chat track
-- more than one pull request. Both columns are NOT NULL with a
-- default of '', and the old key allowed one row per chat, so
-- existing rows already obey the new key.
ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id, git_remote_origin, git_branch);
