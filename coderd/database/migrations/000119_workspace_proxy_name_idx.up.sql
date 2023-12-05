-- No one is using this feature yet as of writing this migration, so this is
-- fine. Just delete all workspace proxies to prevent the new index from having
-- conflicts.
DELETE FROM workspace_proxies;

DROP INDEX IF EXISTS workspace_proxies_name_idx;
CREATE UNIQUE INDEX workspace_proxies_lower_name_idx ON workspace_proxies USING btree (lower(name)) WHERE deleted = FALSE;
