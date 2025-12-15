ALTER TABLE organizations
    ADD COLUMN workspace_sharing_disabled boolean NOT NULL DEFAULT false;
