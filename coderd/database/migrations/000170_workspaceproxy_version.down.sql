BEGIN;
ALTER TABLE workspace_proxies DROP COLUMN version;
COMMIT;
