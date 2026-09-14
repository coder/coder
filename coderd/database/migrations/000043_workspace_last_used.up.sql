ALTER TABLE workspaces
    ADD COLUMN last_used_at timestamp NOT NULL DEFAULT '0001-01-01 00:00:00+00:00';
