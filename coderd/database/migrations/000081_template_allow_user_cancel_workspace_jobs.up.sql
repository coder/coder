ALTER TABLE templates ADD COLUMN allow_user_cancel_workspace_jobs boolean NOT NULL DEFAULT true;

COMMENT ON COLUMN templates.allow_user_cancel_workspace_jobs
IS 'Allow users to cancel in-progress workspace jobs.';
