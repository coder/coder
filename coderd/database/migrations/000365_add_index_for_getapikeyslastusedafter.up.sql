CREATE INDEX api_keys_last_used_idx ON api_keys (last_used DESC);
COMMENT ON INDEX api_keys_last_used_idx IS 'Index for optimizing api_keys queries filtering by last_used';
