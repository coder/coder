DROP INDEX IF EXISTS workspace_proxies_lower_name_idx;

-- Enforces no active proxies have the same name.
CREATE UNIQUE INDEX ON workspace_proxies (name) WHERE deleted = FALSE;
