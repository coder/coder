-- One row per (chat, origin, branch) so a chat can track multiple
-- pull requests. Existing rows satisfy the new key: both columns are
-- NOT NULL with '' defaults and the old key capped rows at one per
-- chat, so no dedup or rewrite is needed. ADD PRIMARY KEY cannot run
-- CONCURRENTLY because migrations run in a single transaction, which
-- is acceptable for this low-volume table. Finish old coderd
-- instances before migrating: they run ON CONFLICT (chat_id), which
-- has no arbiter under the new key.
ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id, git_remote_origin, git_branch);
