ALTER TABLE workspace_agent_devcontainers
  ADD COLUMN build_cache_from text[] NOT NULL DEFAULT '{}';

ALTER TABLE workspace_agent_devcontainers
  ALTER COLUMN build_cache_from DROP DEFAULT;

COMMENT ON COLUMN workspace_agent_devcontainers.build_cache_from
  IS 'External images to use as potential layer cache during devcontainer builds';
