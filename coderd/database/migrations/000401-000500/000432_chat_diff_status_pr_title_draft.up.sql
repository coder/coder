ALTER TABLE chat_diff_statuses ADD COLUMN pull_request_title TEXT NOT NULL DEFAULT '';
ALTER TABLE chat_diff_statuses ADD COLUMN pull_request_draft BOOLEAN NOT NULL DEFAULT FALSE;
