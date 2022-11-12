ALTER TABLE workspace_builds ADD COLUMN cost int NOT NULL DEFAULT 0;
ALTER TABLE workspace_resources ADD COLUMN cost int NOT NULL DEFAULT 0;
ALTER TABLE groups ADD COLUMN quota_allowance int NOT NULL DEFAULT 0;
