-- Remote URLs can carry a token, such as https://<token>@github.com/o/r.git.
-- Strip it with the same pattern as originCredentials in coderd/x/gitsync.
-- Each chat still has one row here, so this cannot create duplicate keys.
UPDATE chat_diff_statuses
SET git_remote_origin = regexp_replace(git_remote_origin, '^(https?://)[^/]*@', '\1')
WHERE git_remote_origin ~ '^https?://[^/]*@';

-- One row per chat, origin, and branch. This lets a chat track
-- more than one pull request. Both columns are NOT NULL with a
-- default of '', and the old key allowed one row per chat, so
-- existing rows already obey the new key.
ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id, git_remote_origin, git_branch);
