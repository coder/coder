-- Drop the foreign key so that boundary logs can be inserted before
-- the session row exists. The session is created lazily and may fail
-- on transient errors; removing the FK lets logs persist regardless.
-- The session row will be created on a subsequent batch, retroactively
-- linking the orphaned logs via session_id.
ALTER TABLE boundary_logs DROP CONSTRAINT boundary_logs_session_id_fkey;
