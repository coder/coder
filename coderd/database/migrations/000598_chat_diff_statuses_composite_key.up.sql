-- One row per chat, origin, and branch. This lets a chat track
-- more than one pull request. Both columns are NOT NULL with a
-- default of '', and the old key allowed one row per chat, so
-- existing rows already obey the new key.
ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id, git_remote_origin, git_branch);

-- When the agent last reported the ref. The newest report is the
-- chat's primary.
ALTER TABLE chat_diff_statuses
    ADD COLUMN reported_at timestamp with time zone DEFAULT now() NOT NULL;

UPDATE chat_diff_statuses
SET reported_at = updated_at;
