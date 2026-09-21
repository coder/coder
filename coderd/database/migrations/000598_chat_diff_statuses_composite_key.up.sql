-- One row per chat, origin, and branch. This lets a chat track
-- more than one pull request. Both columns are NOT NULL with a
-- default of '', and the old key allowed one row per chat, so
-- existing rows already obey the new key.
ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id, git_remote_origin, git_branch);

-- Primary ordering follows the last report, not the last write.
-- updated_at is the generic mutation time, so refreshes and URL
-- backfills must not reorder the primary through it.
ALTER TABLE chat_diff_statuses
    ADD COLUMN reported_at timestamp with time zone DEFAULT now() NOT NULL;

UPDATE chat_diff_statuses
SET reported_at = updated_at;
