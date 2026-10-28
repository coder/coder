ALTER TABLE foundations
    ADD COLUMN prevent_destroy_with_workspaces boolean NOT NULL DEFAULT true;
