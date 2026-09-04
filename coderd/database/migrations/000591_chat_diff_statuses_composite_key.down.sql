-- The old key allows one row per chat, so keep the most recently
-- updated row and drop the rest.
DELETE FROM chat_diff_statuses a
USING chat_diff_statuses b
WHERE a.chat_id = b.chat_id
    AND (a.updated_at, a.git_remote_origin, a.git_branch)
        < (b.updated_at, b.git_remote_origin, b.git_branch);

ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id);
