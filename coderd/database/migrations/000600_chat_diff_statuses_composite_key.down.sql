-- The old key allows one row per chat, so keep the primary row and
-- drop the rest. The primary is the first row of the same ordering
-- the API uses: newest report first, then origin and branch.
DELETE FROM chat_diff_statuses
WHERE (chat_id, git_remote_origin, git_branch) IN (
    SELECT chat_id, git_remote_origin, git_branch
    FROM (
        SELECT
            chat_id,
            git_remote_origin,
            git_branch,
            ROW_NUMBER() OVER (
                PARTITION BY chat_id
                ORDER BY reported_at DESC, git_remote_origin, git_branch
            ) AS rank
        FROM chat_diff_statuses
    ) ranked
    WHERE ranked.rank > 1
);

ALTER TABLE chat_diff_statuses
    DROP CONSTRAINT chat_diff_statuses_pkey;

ALTER TABLE chat_diff_statuses
    ADD PRIMARY KEY (chat_id);

ALTER TABLE chat_diff_statuses
    DROP COLUMN reported_at;
