-- Delete orphaned logs that have no matching session before restoring
-- the FK constraint.
DELETE FROM boundary_logs bl
WHERE NOT EXISTS (
    SELECT 1 FROM boundary_sessions bs WHERE bs.id = bl.session_id
);

ALTER TABLE boundary_logs
    ADD CONSTRAINT boundary_logs_session_id_fkey
    FOREIGN KEY (session_id) REFERENCES boundary_sessions(id) ON DELETE CASCADE;
