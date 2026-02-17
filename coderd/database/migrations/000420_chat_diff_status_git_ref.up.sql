ALTER TABLE chat_diff_statuses
    ADD COLUMN git_branch TEXT NOT NULL DEFAULT '',
    ADD COLUMN git_remote_origin TEXT NOT NULL DEFAULT '',
    DROP COLUMN pull_request_open;

ALTER TABLE chat_diff_statuses
    RENAME COLUMN github_pr_url TO url;

ALTER TABLE chat_diff_statuses
    ALTER COLUMN pull_request_state DROP NOT NULL,
    ALTER COLUMN pull_request_state DROP DEFAULT,
    ALTER COLUMN pull_request_state TYPE TEXT;
