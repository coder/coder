-- One row per (chat, origin, branch) instead of one row per chat, so a
-- chat can track multiple pull requests. git_branch and git_remote_origin
-- are NOT NULL with '' defaults and the old primary key already limited
-- rows to one per chat, so every existing row satisfies the new key and no
-- rewrite or dedup is needed. The index build takes an ACCESS EXCLUSIVE
-- lock for its duration; chat_diff_statuses is a low-volume table, and
-- CREATE INDEX CONCURRENTLY is not an option because every migration runs
-- inside a single transaction. Deployments must finish old coderd
-- instances before migrating: they run ON CONFLICT (chat_id), which has
-- no matching arbiter once the old key is gone.
ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id, git_remote_origin, git_branch);
