CREATE UNIQUE INDEX IF NOT EXISTS tasks_owner_id_name_unique_idx ON tasks (owner_id, LOWER(name)) WHERE deleted_at IS NULL;
COMMENT ON INDEX tasks_owner_id_name_unique_idx IS 'Index to ensure uniqueness for task owner/name';
