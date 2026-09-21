ALTER TABLE chat_projects ADD COLUMN memory_consolidated_at timestamptz;
COMMENT ON COLUMN chat_projects.memory_consolidated_at IS 'When project memory was last consolidated at the cap; rate-limits the next run.';
