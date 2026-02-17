ALTER TABLE chat_diff_statuses
    ALTER COLUMN pull_request_state SET NOT NULL,
    ALTER COLUMN pull_request_state SET DEFAULT '';

ALTER TABLE chat_diff_statuses
    RENAME COLUMN url TO github_pr_url;

ALTER TABLE chat_diff_statuses
    ADD COLUMN pull_request_open BOOLEAN NOT NULL DEFAULT FALSE,
    DROP COLUMN IF EXISTS git_branch,
    DROP COLUMN IF EXISTS git_remote_origin;
