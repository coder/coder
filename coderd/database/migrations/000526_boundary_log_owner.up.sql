ALTER TABLE boundary_logs ADD COLUMN owner_id UUID;

COMMENT ON COLUMN boundary_logs.owner_id IS 'The ID of the user who owns the workspace. NULL for logs inserted before this column existed or if the user was deleted.';

-- Backfill from sessions where possible.
UPDATE boundary_logs bl
SET owner_id = bs.owner_id
FROM boundary_sessions bs
WHERE bl.session_id = bs.id
  AND bs.owner_id IS NOT NULL;

ALTER TABLE boundary_logs
    ADD CONSTRAINT boundary_logs_owner_id_fkey
    FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE SET NULL;
