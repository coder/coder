BEGIN;
ALTER TABLE workspaces ADD COLUMN locked_at timestamptz NULL;
COMMIT;
